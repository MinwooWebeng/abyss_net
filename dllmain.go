package main

import "C"
import (
	"context"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"runtime/cgo"
	"time"

	abyss_and "abyss_neighbor_discovery/and"
	"abyss_neighbor_discovery/aurl"
	abyss_host "abyss_neighbor_discovery/host"
	abyss "abyss_neighbor_discovery/interfaces"
	abyss_net "abyss_neighbor_discovery/net_service"
	"abyss_neighbor_discovery/tools/functional"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/crypto/ssh"
)

const version = "0.9.0"

// return value (C.int)
const (
	EOF               = -1
	ERROR             = -1
	INVALID_ARGUMENTS = -2
	BUFFER_OVERFLOW   = -3
	INVALID_HANDLE    = -99 //for method calls
)

//export GetVersion
func GetVersion(buf *C.char, buflen C.int) C.int {
	return TryMarshalBytes(buf, buflen, []byte(version))
}

var error_queue chan error

//export PopErrorQueue
func PopErrorQueue() C.uintptr_t {
	select {
	case err := <-error_queue:
		return C.uintptr_t(cgo.NewHandle(err))
	default:
		return C.uintptr_t(cgo.NewHandle(errors.New("no error")))
	}
}

//export GetErrorBodyLength
func GetErrorBodyLength(h_error C.uintptr_t) C.int {
	err := (cgo.Handle(h_error)).Value().(error)
	return C.int(len(err.Error()))
}

//export GetErrorBody
func GetErrorBody(h_error C.uintptr_t, buf *C.char, buflen C.int) C.int {
	err := (cgo.Handle(h_error)).Value().(error)
	return TryMarshalBytes(buf, buflen, []byte(err.Error()))
}

type IDestructable interface {
	Destuct()
}

//export CloseHandle
func CloseHandle(handle C.uintptr_t) {
	inner := cgo.Handle(handle).Value()
	if inner_decon, ok := inner.(IDestructable); ok {
		inner_decon.Destuct()
	}
	cgo.Handle(handle).Delete()
}

//export NewSimplePathResolver
func NewSimplePathResolver() C.uintptr_t {
	return C.uintptr_t(cgo.NewHandle(abyss_host.NewSimplePathResolver()))
}

//export SimplePathResolver_SetMapping
func SimplePathResolver_SetMapping(h C.uintptr_t, path_ptr *C.char, path_len C.int, world_ID_out *C.char) C.int {
	path_resolver, ok := cgo.Handle(h).Value().(*abyss_host.SimplePathResolver)
	if !ok {
		return INVALID_HANDLE
	}
	var world_uuid uuid.UUID
	data := UnmarshalBytes(world_ID_out, 16)
	copy(world_uuid[:], data)
	path_resolver.SetMapping(string(UnmarshalBytes(path_ptr, path_len)), world_uuid)
	return 0
}

//export SimplePathResolver_DeleteMapping
func SimplePathResolver_DeleteMapping(h C.uintptr_t, path_ptr *C.char, path_len C.int) C.int {
	path_resolver, ok := cgo.Handle(h).Value().(*abyss_host.SimplePathResolver)
	if !ok {
		return INVALID_HANDLE
	}
	path_resolver.DeleteMapping(string(UnmarshalBytes(path_ptr, path_len)))
	return 0
}

//export NewHost
func NewHost(root_priv_key_pem_ptr *C.char, root_priv_key_pem_len C.int, h_path_resolver C.uintptr_t) C.uintptr_t {
	root_priv_key_pem := UnmarshalBytes(root_priv_key_pem_ptr, root_priv_key_pem_len)
	root_priv_key_block, _ := pem.Decode(root_priv_key_pem)

	if root_priv_key_block == nil {
		error_queue <- errors.New("invalid private key PEM block")
		return 0
	}

	root_priv_key, err := ssh.ParseRawPrivateKey(root_priv_key_block.Bytes)
	if err != nil {
		error_queue <- err
		return 0
	}
	root_priv_key_casted, ok := root_priv_key.(abyss_net.PrivateKey)
	if !ok {
		error_queue <- errors.New("unsupported private key type")
		return 0
	}

	path_resolver, ok := cgo.Handle(h_path_resolver).Value().(*abyss_host.SimplePathResolver)
	if !ok {
		error_queue <- errors.New("invalid handle for path resolver")
	}

	net_service, err := abyss_net.NewBetaNetService(root_priv_key_casted, abyss_net.NewBetaAddressSelector())
	if err != nil {
		error_queue <- err
		return 0
	}

	host := abyss_host.NewAbyssHost(
		net_service,
		abyss_and.NewAND(net_service.LocalIdentity().IDHash()),
		path_resolver,
	)
	go host.ListenAndServe(context.Background())

	return C.uintptr_t(cgo.NewHandle(host))
}

//export Host_GetLocalAbyssURL
func Host_GetLocalAbyssURL(h C.uintptr_t, buf *C.char, buflen C.int) C.int {
	host, ok := cgo.Handle(h).Value().(*abyss_host.AbyssHost)
	if !ok {
		return INVALID_HANDLE
	}

	return TryMarshalBytes(buf, buflen, []byte(host.GetLocalAbyssURL().ToString()))
}

//export Host_OpenOutboundConnection
func Host_OpenOutboundConnection(h C.uintptr_t, abyss_url_ptr *C.char, abyss_url_len C.int) C.int {
	host, ok := cgo.Handle(h).Value().(*abyss_host.AbyssHost)
	if !ok {
		return INVALID_HANDLE
	}

	aurl, err := aurl.ParseAURL(string(UnmarshalBytes(abyss_url_ptr, abyss_url_len)))
	if err != nil {
		return INVALID_ARGUMENTS
	}
	host.OpenOutboundConnection(aurl)
	return 0
}

type WorldExport struct {
	inner    abyss.IAbyssWorld
	origin   abyss.IAbyssHost
	event_ch chan any
}

func (w *WorldExport) Destruct() {
	w.origin.LeaveWorld(w.inner)
}

//export Host_OpenWorld
func Host_OpenWorld(h C.uintptr_t, url_ptr *C.char, url_len C.int) C.uintptr_t {
	host, ok := cgo.Handle(h).Value().(*abyss_host.AbyssHost)
	if !ok {
		error_queue <- errors.New("invalid handle")
		return 0
	}

	world, err := host.OpenWorld(string(UnmarshalBytes(url_ptr, url_len)))
	if err != nil {
		error_queue <- err
		return 0
	}

	return C.uintptr_t(cgo.NewHandle(&WorldExport{
		inner:    world,
		origin:   host,
		event_ch: world.GetEventChannel(),
	}))
}

//export Host_JoinWorld
func Host_JoinWorld(h C.uintptr_t, url_ptr *C.char, url_len C.int, timeout_ms C.int) C.uintptr_t {
	host, ok := cgo.Handle(h).Value().(*abyss_host.AbyssHost)
	if !ok {
		error_queue <- errors.New("invalid handle")
		return 0
	}

	aurl, err := aurl.ParseAURL(string(UnmarshalBytes(url_ptr, url_len)))
	if err != nil {
		error_queue <- err
		return 0
	}

	ctx, ctx_cancel := context.WithTimeout(context.Background(), time.Duration(timeout_ms)*time.Millisecond)
	defer ctx_cancel()
	world, err := host.JoinWorld(ctx, aurl)
	if err != nil {
		error_queue <- err
		return 0
	}

	return C.uintptr_t(cgo.NewHandle(&WorldExport{
		inner:    world,
		origin:   host,
		event_ch: world.GetEventChannel(),
	}))
}

func World_GetSessionID(h C.uintptr_t, world_ID_out *C.char) C.int {
	world, ok := cgo.Handle(h).Value().(*WorldExport)
	if !ok {
		return INVALID_HANDLE
	}
	dest := UnmarshalBytes(world_ID_out, 16)
	world_ID := world.inner.SessionID()
	copy(dest, world_ID[:])
	return 0
}

// TODO: change this to full json interfaces.
//
//export World_WaitEvent
func World_WaitEvent(h C.uintptr_t, event_type_out *C.char) C.uintptr_t {
	world, ok := cgo.Handle(h).Value().(*WorldExport)
	if !ok {
		error_queue <- errors.New("invalid handle")
		return 0
	}

	event_any := <-world.event_ch

	switch event := event_any.(type) {
	case abyss.EWorldPeerRequest:
		*event_type_out = 1
		return C.uintptr_t(cgo.NewHandle(&event))
	case abyss.EWorldPeerReady:
		*event_type_out = 2
		return C.uintptr_t(cgo.NewHandle(event.Peer))
	case abyss.EPeerObjectAppend:
		*event_type_out = 3
		return C.uintptr_t(cgo.NewHandle(&event))
	case abyss.EPeerObjectDelete:
		*event_type_out = 4
		return C.uintptr_t(cgo.NewHandle(&event))
	case abyss.EWorldPeerLeave:
		*event_type_out = 5
		return C.uintptr_t(cgo.NewHandle(&event))
	case abyss.EWorldTerminate:
		*event_type_out = 6
		return 0
	default:
		error_queue <- errors.New("internal fault")
		*event_type_out = -1
		return 0
	}
}

//export WorldPeerRequest_Accept
func WorldPeerRequest_Accept(h C.uintptr_t) C.int {
	event, ok := cgo.Handle(h).Value().(*abyss.EWorldPeerRequest)
	if !ok {
		return INVALID_HANDLE
	}

	event.Accept()
	return 0
}

//export WorldPeerRequest_Decline
func WorldPeerRequest_Decline(h C.uintptr_t, code C.int, msg *C.char, msglen C.int) C.int {
	event, ok := cgo.Handle(h).Value().(*abyss.EWorldPeerRequest)
	if !ok {
		return INVALID_HANDLE
	}

	event.Decline(int(code), string(UnmarshalBytes(msg, msglen)))
	return 0
}

//export WorldPeer_GetHash
func WorldPeer_GetHash(h C.uintptr_t, buf *C.char, buflen C.int) C.int {
	peer, ok := cgo.Handle(h).Value().(abyss.IAbyssPeer)
	if !ok {
		return INVALID_HANDLE
	}

	return TryMarshalBytes(buf, buflen, []byte(peer.Hash()))
}

//export WorldPeer_AppendObjects
func WorldPeer_AppendObjects(h C.uintptr_t, json_ptr *C.char, json_len C.int) C.int {
	peer, ok := cgo.Handle(h).Value().(abyss.IAbyssPeer)
	if !ok {
		return INVALID_HANDLE
	}

	json_data := UnmarshalBytes(json_ptr, json_len)
	var raw_object_infos []struct {
		id   string
		addr string
	}
	err := json.Unmarshal(json_data, &raw_object_infos)
	if err != nil {
		error_queue <- err
		return INVALID_ARGUMENTS
	}
	res, _, err := functional.Filter_until_err(raw_object_infos, func(i struct {
		id   string
		addr string
	}) (abyss.ObjectInfo, error) {
		bytes, err := hex.DecodeString(i.id)
		if err != nil {
			return abyss.ObjectInfo{}, err
		}
		return abyss.ObjectInfo{
			ID:      uuid.UUID(bytes),
			Address: i.addr,
		}, nil
	})
	if err != nil {
		error_queue <- err
		return INVALID_ARGUMENTS
	}

	peer.AppendObjects(res)
	return 0
}

//export WorldPeer_DeleteObjects
func WorldPeer_DeleteObjects(h C.uintptr_t, json_ptr *C.char, json_len C.int) C.int {
	peer, ok := cgo.Handle(h).Value().(abyss.IAbyssPeer)
	if !ok {
		return INVALID_HANDLE
	}

	json_data := UnmarshalBytes(json_ptr, json_len)
	var raw_object_ids []string
	err := json.Unmarshal(json_data, &raw_object_ids)
	if err != nil {
		error_queue <- err
		return INVALID_ARGUMENTS
	}
	res, _, err := functional.Filter_until_err(raw_object_ids, func(i string) (uuid.UUID, error) {
		bytes, err := hex.DecodeString(i)
		if err != nil {
			return uuid.Nil, err
		}
		return uuid.UUID(bytes), nil
	})
	if err != nil {
		error_queue <- err
		return INVALID_ARGUMENTS
	}

	peer.DeleteObjects(res)
	return 0
}

//export WorldPeerLeave_GetHash
func WorldPeerLeave_GetHash(h C.uintptr_t, buf *C.char, buflen C.int) C.int {
	event, ok := cgo.Handle(h).Value().(*abyss.EWorldPeerLeave)
	if !ok {
		return INVALID_HANDLE
	}

	return TryMarshalBytes(buf, buflen, []byte(event.PeerHash))
}

type AbystClientExport struct {
	inner *http3.ClientConn
}

func (c *AbystClientExport) Destuct() {
	c.inner.CloseWithError(0, "abyst client disconnected")
}

//export Host_GetAbystClientConnection
func Host_GetAbystClientConnection(h C.uintptr_t, peer_hash_ptr *C.char, peer_hash_len C.int, timeout_ms C.int) C.uintptr_t {
	host, ok := cgo.Handle(h).Value().(*abyss_host.AbyssHost)
	if !ok {
		error_queue <- errors.New("invalid handle")
		return 0
	}

	ctx, ctx_cancel := context.WithTimeout(context.Background(), time.Duration(timeout_ms)*time.Millisecond)
	defer ctx_cancel()
	http_client, err := host.GetAbystClientConnection(ctx, string(UnmarshalBytes(peer_hash_ptr, peer_hash_len)))
	if err != nil {
		error_queue <- err
		return 0
	}

	return C.uintptr_t(cgo.NewHandle(&AbystClientExport{
		inner: http_client,
	}))
}

type AbystResponseExport struct {
	inner *http.Response
}

func (w *AbystResponseExport) Destruct() {
	w.inner.Body.Close()
}

//export AbystClient_Request
func AbystClient_Request(h C.uintptr_t, method C.int, path_ptr *C.char, path_len C.int) C.uintptr_t {
	client, ok := cgo.Handle(h).Value().(*http3.ClientConn)
	if !ok {
		error_queue <- errors.New("invalid handle")
		return 0
	}
	var method_string string
	switch method {
	case 0:
		method_string = http.MethodGet
	default:
		error_queue <- errors.New("invalid method")
		return 0
	}
	request, err := http.NewRequest(method_string, "https://a.abyst/"+string(UnmarshalBytes(path_ptr, path_len)), nil)
	if err != nil {
		error_queue <- err
		return 0
	}
	response, err := client.RoundTrip(request)
	if err != nil {
		error_queue <- err
		return 0
	}

	return C.uintptr_t(cgo.NewHandle(&AbystResponseExport{
		inner: response,
	}))
}

func AbystResponse_ReadBody(h C.uintptr_t, buf *C.char, buflen C.int) C.int {
	response, ok := cgo.Handle(h).Value().(*AbystResponseExport)
	if !ok {
		return INVALID_HANDLE
	}

	if buflen <= 0 || buflen > 134217728 { //over 100MiB - must be some error.
		return INVALID_ARGUMENTS
	}

	read_len, err := response.inner.Body.Read(UnmarshalBytes(buf, buflen))
	if read_len == 0 && err != nil {
		return EOF
	}

	return C.int(read_len)
}

//TODO: enable some external binding for abyst server. we may expect all abyst local hosts are just available some elsewhere. enable forwarding

func main() {
	//TODO: debug log initialization
	error_queue = make(chan error, 32)
}
