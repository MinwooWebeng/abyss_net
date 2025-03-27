/* Code generated by cmd/cgo; DO NOT EDIT. */

/* package abyss_neighbor_discovery */


#line 1 "cgo-builtin-export-prolog"

#include <stddef.h>

#ifndef GO_CGO_EXPORT_PROLOGUE_H
#define GO_CGO_EXPORT_PROLOGUE_H

#ifndef GO_CGO_GOSTRING_TYPEDEF
typedef struct { const char *p; ptrdiff_t n; } _GoString_;
#endif

#endif

/* Start of preamble from import "C" comments.  */




/* End of preamble from import "C" comments.  */


/* Start of boilerplate cgo prologue.  */
#line 1 "cgo-gcc-export-header-prolog"

#ifndef GO_CGO_PROLOGUE_H
#define GO_CGO_PROLOGUE_H

typedef signed char GoInt8;
typedef unsigned char GoUint8;
typedef short GoInt16;
typedef unsigned short GoUint16;
typedef int GoInt32;
typedef unsigned int GoUint32;
typedef long long GoInt64;
typedef unsigned long long GoUint64;
typedef GoInt64 GoInt;
typedef GoUint64 GoUint;
typedef size_t GoUintptr;
typedef float GoFloat32;
typedef double GoFloat64;
#ifdef _MSC_VER
#include <complex.h>
typedef _Fcomplex GoComplex64;
typedef _Dcomplex GoComplex128;
#else
typedef float _Complex GoComplex64;
typedef double _Complex GoComplex128;
#endif

/*
  static assertion to make sure the file is being used on architecture
  at least with matching size of GoInt.
*/
typedef char _check_for_64_bit_pointer_matching_GoInt[sizeof(void*)==64/8 ? 1:-1];

#ifndef GO_CGO_GOSTRING_TYPEDEF
typedef _GoString_ GoString;
#endif
typedef void *GoMap;
typedef void *GoChan;
typedef struct { void *t; void *v; } GoInterface;
typedef struct { void *data; GoInt len; GoInt cap; } GoSlice;

#endif

/* End of boilerplate cgo prologue.  */

#ifdef __cplusplus
extern "C" {
#endif

extern __declspec(dllexport) int GetVersion(char* buf, int buflen);
extern __declspec(dllexport) uintptr_t PopErrorQueue();
extern __declspec(dllexport) int GetErrorBodyLength(uintptr_t h_error);
extern __declspec(dllexport) int GetErrorBody(uintptr_t h_error, char* buf, int buflen);
extern __declspec(dllexport) void CloseHandle(uintptr_t handle);
extern __declspec(dllexport) uintptr_t NewSimplePathResolver();
extern __declspec(dllexport) int SimplePathResolver_SetMapping(uintptr_t h, char* path_ptr, int path_len, char* world_ID_out);
extern __declspec(dllexport) int SimplePathResolver_DeleteMapping(uintptr_t h, char* path_ptr, int path_len);
extern __declspec(dllexport) uintptr_t NewHost(char* root_priv_key_pem_ptr, int root_priv_key_pem_len, uintptr_t h_path_resolver);
extern __declspec(dllexport) int Host_GetLocalAbyssURL(uintptr_t h, char* buf, int buflen);
extern __declspec(dllexport) int Host_OpenOutboundConnection(uintptr_t h, char* abyss_url_ptr, int abyss_url_len);
extern __declspec(dllexport) uintptr_t Host_OpenWorld(uintptr_t h, char* url_ptr, int url_len);
extern __declspec(dllexport) uintptr_t Host_JoinWorld(uintptr_t h, char* url_ptr, int url_len, int timeout_ms);

// TODO: change this to full json interfaces.
//
extern __declspec(dllexport) uintptr_t World_WaitEvent(uintptr_t h, char* event_type_out);
extern __declspec(dllexport) int WorldPeerRequest_Accept(uintptr_t h);
extern __declspec(dllexport) int WorldPeerRequest_Decline(uintptr_t h, int code, char* msg, int msglen);
extern __declspec(dllexport) int WorldPeer_GetHash(uintptr_t h, char* buf, int buflen);
extern __declspec(dllexport) int WorldPeer_AppendObjects(uintptr_t h, char* json_ptr, int json_len);
extern __declspec(dllexport) int WorldPeer_DeleteObjects(uintptr_t h, char* json_ptr, int json_len);
extern __declspec(dllexport) int WorldPeerLeave_GetHash(uintptr_t h, char* buf, int buflen);
extern __declspec(dllexport) uintptr_t Host_GetAbystClientConnection(uintptr_t h, char* peer_hash_ptr, int peer_hash_len, int timeout_ms);
extern __declspec(dllexport) uintptr_t AbystClient_Request(uintptr_t h, int method, char* path_ptr, int path_len);

#ifdef __cplusplus
}
#endif
