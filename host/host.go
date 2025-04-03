package host

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	"abyss_neighbor_discovery/ahmp"
	"abyss_neighbor_discovery/and"
	"abyss_neighbor_discovery/aurl"
	abyss "abyss_neighbor_discovery/interfaces"
	"abyss_neighbor_discovery/tools/functional"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

type WorldCreationEvent struct {
	ok      bool
	code    int
	message string
	world   *World
}

type AbyssHost struct {
	ctx         context.Context //set at ListenAndServe(ctx)
	listen_done chan bool
	event_done  chan bool

	NetworkService             abyss.INetworkService
	neighborDiscoveryAlgorithm abyss.INeighborDiscovery
	pathResolver               abyss.IPathResolver

	abystClientTr *http3.Transport

	worlds     map[uuid.UUID]*World
	worlds_mtx *sync.Mutex

	join_queue map[uuid.UUID]chan *WorldCreationEvent //forwarding of AND join result event.
	join_q_mtx *sync.Mutex
}

func NewAbyssHost(netServ abyss.INetworkService, nda abyss.INeighborDiscovery, path_resolver abyss.IPathResolver) *AbyssHost {
	return &AbyssHost{
		listen_done:                make(chan bool, 1),
		event_done:                 make(chan bool, 1),
		NetworkService:             netServ,
		neighborDiscoveryAlgorithm: nda,
		pathResolver:               path_resolver,
		abystClientTr: &http3.Transport{
			Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
				return nil, errors.New("dialing in abyst transport is prohibited")
			},
		},
		worlds:     make(map[uuid.UUID]*World),
		worlds_mtx: new(sync.Mutex),
		join_queue: make(map[uuid.UUID]chan *WorldCreationEvent),
		join_q_mtx: new(sync.Mutex),
	}
}

func (h *AbyssHost) GetLocalAbyssURL() *aurl.AURL {
	origin := h.NetworkService.LocalAURL()
	return &aurl.AURL{
		Scheme: origin.Scheme,
		Hash:   origin.Hash,
		Addresses: functional.Accum_all(
			origin.Addresses,
			make([]*net.UDPAddr, 0, len(origin.Addresses)),
			func(addr *net.UDPAddr, acc []*net.UDPAddr) []*net.UDPAddr {
				return append(acc, addr)
			},
		),
		Path: origin.Path,
	}
}

func (h *AbyssHost) OpenOutboundConnection(abyss_url *aurl.AURL) {
	h.NetworkService.ConnectAbyssAsync(h.ctx, abyss_url)
}

func (h *AbyssHost) OpenWorld(world_url string) (abyss.IAbyssWorld, error) {
	//open is now equally treated with join event
	join_res_ch := make(chan *WorldCreationEvent, 1)

	local_session_id := uuid.New()

	h.join_q_mtx.Lock()
	h.join_queue[local_session_id] = join_res_ch
	h.join_q_mtx.Unlock()

	retval := h.neighborDiscoveryAlgorithm.OpenWorld(local_session_id, world_url)
	if retval == abyss.EINVAL {
		return nil, errors.New("OpenWorld: invalid arguments")
	} else if retval == abyss.EPANIC {
		panic("fatal:::AND corrupted while opening world")
	}

	if err := h.neighborDiscoveryAlgorithm.(*and.AND).CheckSanity(); err != nil {
		panic("AND sanity check failed!!! :: " + err.Error())
	}

	//wait for join result.
	join_res := <-join_res_ch

	if !join_res.ok {
		panic("world open failed unexpetedly")
	}

	return join_res.world, nil
}
func (h *AbyssHost) JoinWorld(ctx context.Context, abyss_url *aurl.AURL) (abyss.IAbyssWorld, error) {
	local_session_id := uuid.New()

	retval := h.neighborDiscoveryAlgorithm.JoinWorld(local_session_id, abyss_url)
	if retval == abyss.EINVAL {
		return nil, errors.New("failed to join world::unknown error")
	} else if retval == abyss.EPANIC {
		panic("fatal:::AND corrupted while joining world")
	}

	if err := h.neighborDiscoveryAlgorithm.(*and.AND).CheckSanity(); err != nil {
		panic("AND sanity check failed!!! :: " + err.Error())
	}

	join_res_ch := make(chan *WorldCreationEvent, 1)

	h.join_q_mtx.Lock()
	h.join_queue[local_session_id] = join_res_ch
	h.join_q_mtx.Unlock()

	ctx_done_waiter := make(chan bool, 1)
	go func() { //context watchdog
		select {
		case <-ctx.Done():
			h.neighborDiscoveryAlgorithm.CancelJoin(local_session_id) //this request AND module to early-return join failure

			if err := h.neighborDiscoveryAlgorithm.(*and.AND).CheckSanity(); err != nil {
				panic("AND sanity check failed!!! :: " + err.Error())
			}
		case <-ctx_done_waiter:
			return
		}
	}()

	//wait for join result.
	join_res := <-join_res_ch

	//as join result arrived, kill the context watchdog goroutine.
	ctx_done_waiter <- true

	if !join_res.ok {
		return nil, errors.New(join_res.message)
	}

	return join_res.world, nil
}
func (h *AbyssHost) LeaveWorld(world abyss.IAbyssWorld) {
	if h.neighborDiscoveryAlgorithm.CloseWorld(world.SessionID()) != 0 {
		panic("World Leave failed")
	}

	if err := h.neighborDiscoveryAlgorithm.(*and.AND).CheckSanity(); err != nil {
		panic("AND sanity check failed!!! :: " + err.Error())
	}
}

func (h *AbyssHost) GetAbystClientConnection(ctx context.Context, peer_hash string) (*http3.ClientConn, error) {
	conn, err := h.NetworkService.ConnectAbyst(ctx, peer_hash)
	if err != nil {
		return nil, err
	}
	return h.abystClientTr.NewClientConn(conn), nil
}
func (h *AbyssHost) GetAbystServerPeerChannel() chan abyss.AbystInboundSession {
	return h.NetworkService.GetAbystServerPeerChannel()
}

func (h *AbyssHost) ListenAndServe(ctx context.Context) {
	if h.ctx != nil {
		panic("ListenAndServe called twice")
	}
	h.ctx = ctx

	net_done := make(chan bool, 1)
	go func() {
		if err := h.NetworkService.ListenAndServe(ctx); err != nil {
			fmt.Println(time.Now().Format("00:00:00.000") + "[network service failed] " + err.Error())
		}
		net_done <- true
	}()
	go h.listenLoop()
	go h.eventLoop()

	<-h.listen_done
	<-h.event_done

	<-net_done
}

func (h *AbyssHost) listenLoop() {
	var wg sync.WaitGroup

	accept_ch := h.NetworkService.GetAbyssPeerChannel()
	for {
		select {
		case <-h.ctx.Done():
			wg.Wait()
			h.listen_done <- true
			return
		case peer := <-accept_ch:
			wg.Add(1)
			go func() {
				defer wg.Done()
				h.serveLoop(peer)
			}()
		}
	}
}

func (h *AbyssHost) serveLoop(peer abyss.IANDPeer) {
	retval := h.neighborDiscoveryAlgorithm.PeerConnected(peer)
	if retval != 0 {
		return
	}

	if err := h.neighborDiscoveryAlgorithm.(*and.AND).CheckSanity(); err != nil {
		panic("AND sanity check failed!!! :: " + err.Error())
	}

	ahmp_channel := peer.AhmpCh()
	for {
		select {
		case <-h.ctx.Done():
			return
		case message_any := <-ahmp_channel:
			var and_result abyss.ANDERROR

			switch message := message_any.(type) {
			case *ahmp.JN:
				local_session_id, ok := h.pathResolver.PathToSessionID(message.Text, peer.IDHash())
				if !ok {
					continue // TODO: respond with proper error code
				}
				and_result = h.neighborDiscoveryAlgorithm.JN(local_session_id, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID})
			case *ahmp.JOK:
				and_result = h.neighborDiscoveryAlgorithm.JOK(message.RecverSessionID, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID}, message.Text, message.Neighbors)
			case *ahmp.JDN:
				and_result = h.neighborDiscoveryAlgorithm.JDN(message.RecverSessionID, peer, message.Code, message.Text)
			case *ahmp.JNI:
				and_result = h.neighborDiscoveryAlgorithm.JNI(message.RecverSessionID, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID}, message.Neighbor)
			case *ahmp.MEM:
				and_result = h.neighborDiscoveryAlgorithm.MEM(message.RecverSessionID, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID})
			case *ahmp.SNB:
				and_result = h.neighborDiscoveryAlgorithm.SNB(message.RecverSessionID, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID}, message.MemberInfos)
			case *ahmp.CRR:
				and_result = h.neighborDiscoveryAlgorithm.CRR(message.RecverSessionID, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID}, message.MemberInfos)
			case *ahmp.RST:
				and_result = h.neighborDiscoveryAlgorithm.RST(message.RecverSessionID, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID})
			case *ahmp.SOA:
				and_result = h.neighborDiscoveryAlgorithm.SOA(message.RecverSessionID, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID}, message.Objects)
			case *ahmp.SOD:
				and_result = h.neighborDiscoveryAlgorithm.SOD(message.RecverSessionID, abyss.ANDPeerSession{Peer: peer, PeerSessionID: message.SenderSessionID}, message.ObjectIDs)
			default:
				panic("unknown ahmp message type")
			}

			if and_result == abyss.EPANIC {
				panic("AND panic!!!")
			} else if and_result == abyss.EINVAL {
				fmt.Println("AND: invalid arguments - " + reflect.TypeOf(message_any).String() + fmt.Sprintf("%+v", message_any))
			}

			if err := h.neighborDiscoveryAlgorithm.(*and.AND).CheckSanity(); err != nil {
				panic("AND sanity check failed!!! :: " + err.Error())
			}
		}
	}
}

func (h *AbyssHost) eventLoop() {
	event_ch := h.neighborDiscoveryAlgorithm.EventChannel()

	var wg sync.WaitGroup

	for {
		select {
		case <-h.ctx.Done():
			fmt.Println("host event loop done")
			wg.Wait()
			h.event_done <- true
			return
		case e := <-event_ch:
			switch e.Type {
			case abyss.ANDSessionRequest:
				//fmt.Println("event ::: abyss.ANDSessionRequest")
				h.worlds_mtx.Lock()
				world, ok := h.worlds[e.LocalSessionID]
				h.worlds_mtx.Unlock()

				if !ok {
					panic("world not found")
				}

				world.RaisePeerRequest(abyss.ANDPeerSession{
					Peer:          e.Peer,
					PeerSessionID: e.PeerSessionID,
				})
			case abyss.ANDSessionReady:
				//fmt.Println("event ::: abyss.ANDSessionReady")
				h.worlds_mtx.Lock()
				world, ok := h.worlds[e.LocalSessionID]
				h.worlds_mtx.Unlock()

				if !ok {
					panic("world not found")
				}

				world.RaisePeerReady(abyss.ANDPeerSession{
					Peer:          e.Peer,
					PeerSessionID: e.PeerSessionID,
				})
			case abyss.ANDSessionClose:
				//fmt.Println("event ::: abyss.ANDSessionClose")
				h.worlds_mtx.Lock()
				world, ok := h.worlds[e.LocalSessionID]
				h.worlds_mtx.Unlock()

				if !ok {
					panic("world not found")
				}

				world.RaisePeerLeave(e.Peer.IDHash())
			case abyss.ANDJoinSuccess, abyss.ANDJoinFail:
				//fmt.Println("event ::: abyss.ANDJoinResponse")

				var new_world *World
				if e.Type == abyss.ANDJoinSuccess {
					new_world = NewWorld(h.neighborDiscoveryAlgorithm, e.LocalSessionID, e.Text)
					h.worlds_mtx.Lock()
					h.worlds[e.LocalSessionID] = new_world
					h.worlds_mtx.Unlock()
				}

				h.join_q_mtx.Lock()
				join_res_ch := h.join_queue[e.LocalSessionID]
				delete(h.join_queue, e.LocalSessionID)
				h.join_q_mtx.Unlock()

				join_res_ch <- &WorldCreationEvent{
					ok:      e.Type == abyss.ANDJoinSuccess,
					code:    e.Value,
					message: e.Text,
					world:   new_world,
				}
			case abyss.ANDWorldLeave:
				h.worlds_mtx.Lock()
				world, ok := h.worlds[e.LocalSessionID]
				delete(h.worlds, e.LocalSessionID)
				h.worlds_mtx.Unlock()

				if !ok {
					panic("world not found")
				}

				world.RaiseWorldTerminate()
			case abyss.ANDConnectRequest:
				//fmt.Println("event ::: abyss.ANDConnectRequest")
				h.NetworkService.ConnectAbyssAsync(h.ctx, e.Object.(*aurl.AURL))
			case abyss.ANDTimerRequest:
				target_local_session := e.LocalSessionID
				duration := e.Value
				wg.Add(1)
				go func() {
					defer wg.Done()
					select {
					case <-h.ctx.Done():
					case <-time.NewTimer(time.Duration(duration) * time.Millisecond).C:
						h.neighborDiscoveryAlgorithm.TimerExpire(target_local_session)
					}
				}()
			case abyss.ANDPeerRegister:
				certificates := e.Object.(*abyss.PeerCertificates)
				h.NetworkService.AppendKnownPeerDer(certificates.RootCertDer, certificates.HandshakeKeyCertDer)
			case abyss.ANDObjectAppend:
				h.worlds_mtx.Lock()
				world, ok := h.worlds[e.LocalSessionID]
				h.worlds_mtx.Unlock()

				if !ok {
					panic("world not found")
				}

				world.RaiseObjectAppend(e.Peer.IDHash(), e.Object.([]abyss.ObjectInfo))

			case abyss.ANDObjectDelete:
				h.worlds_mtx.Lock()
				world, ok := h.worlds[e.LocalSessionID]
				h.worlds_mtx.Unlock()

				if !ok {
					panic("world not found")
				}

				world.RaiseObjectDelete(e.Peer.IDHash(), e.Object.([]uuid.UUID))

			case abyss.ANDNeighborEventDebug:
				//fmt.Println("event ::: abyss.ANDNeighborEventDebug")
				fmt.Println(time.Now().Format("00:00:00.000") + " " + e.Text)
			default:
				panic("unknown AND event")
			}
		}
	}
}
