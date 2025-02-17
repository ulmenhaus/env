// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v3.12.4
// source: jql/jql.proto

package jqlpb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	JQL_ListTables_FullMethodName     = "/jql.JQL/ListTables"
	JQL_ListRows_FullMethodName       = "/jql.JQL/ListRows"
	JQL_GetRow_FullMethodName         = "/jql.JQL/GetRow"
	JQL_WriteRow_FullMethodName       = "/jql.JQL/WriteRow"
	JQL_DeleteRow_FullMethodName      = "/jql.JQL/DeleteRow"
	JQL_IncrementEntry_FullMethodName = "/jql.JQL/IncrementEntry"
	JQL_Persist_FullMethodName        = "/jql.JQL/Persist"
	JQL_GetSnapshot_FullMethodName    = "/jql.JQL/GetSnapshot"
	JQL_LoadSnapshot_FullMethodName   = "/jql.JQL/LoadSnapshot"
)

// JQLClient is the client API for JQL service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type JQLClient interface {
	ListTables(ctx context.Context, in *ListTablesRequest, opts ...grpc.CallOption) (*ListTablesResponse, error)
	ListRows(ctx context.Context, in *ListRowsRequest, opts ...grpc.CallOption) (*ListRowsResponse, error)
	GetRow(ctx context.Context, in *GetRowRequest, opts ...grpc.CallOption) (*GetRowResponse, error)
	WriteRow(ctx context.Context, in *WriteRowRequest, opts ...grpc.CallOption) (*WriteRowResponse, error)
	DeleteRow(ctx context.Context, in *DeleteRowRequest, opts ...grpc.CallOption) (*DeleteRowResponse, error)
	IncrementEntry(ctx context.Context, in *IncrementEntryRequest, opts ...grpc.CallOption) (*IncrementEntryResponse, error)
	Persist(ctx context.Context, in *PersistRequest, opts ...grpc.CallOption) (*PersistResponse, error)
	GetSnapshot(ctx context.Context, in *GetSnapshotRequest, opts ...grpc.CallOption) (*GetSnapshotResponse, error)
	LoadSnapshot(ctx context.Context, in *LoadSnapshotRequest, opts ...grpc.CallOption) (*LoadSnapshotResponse, error)
}

type jQLClient struct {
	cc grpc.ClientConnInterface
}

func NewJQLClient(cc grpc.ClientConnInterface) JQLClient {
	return &jQLClient{cc}
}

func (c *jQLClient) ListTables(ctx context.Context, in *ListTablesRequest, opts ...grpc.CallOption) (*ListTablesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListTablesResponse)
	err := c.cc.Invoke(ctx, JQL_ListTables_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jQLClient) ListRows(ctx context.Context, in *ListRowsRequest, opts ...grpc.CallOption) (*ListRowsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListRowsResponse)
	err := c.cc.Invoke(ctx, JQL_ListRows_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jQLClient) GetRow(ctx context.Context, in *GetRowRequest, opts ...grpc.CallOption) (*GetRowResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetRowResponse)
	err := c.cc.Invoke(ctx, JQL_GetRow_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jQLClient) WriteRow(ctx context.Context, in *WriteRowRequest, opts ...grpc.CallOption) (*WriteRowResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(WriteRowResponse)
	err := c.cc.Invoke(ctx, JQL_WriteRow_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jQLClient) DeleteRow(ctx context.Context, in *DeleteRowRequest, opts ...grpc.CallOption) (*DeleteRowResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(DeleteRowResponse)
	err := c.cc.Invoke(ctx, JQL_DeleteRow_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jQLClient) IncrementEntry(ctx context.Context, in *IncrementEntryRequest, opts ...grpc.CallOption) (*IncrementEntryResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(IncrementEntryResponse)
	err := c.cc.Invoke(ctx, JQL_IncrementEntry_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jQLClient) Persist(ctx context.Context, in *PersistRequest, opts ...grpc.CallOption) (*PersistResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(PersistResponse)
	err := c.cc.Invoke(ctx, JQL_Persist_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jQLClient) GetSnapshot(ctx context.Context, in *GetSnapshotRequest, opts ...grpc.CallOption) (*GetSnapshotResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetSnapshotResponse)
	err := c.cc.Invoke(ctx, JQL_GetSnapshot_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jQLClient) LoadSnapshot(ctx context.Context, in *LoadSnapshotRequest, opts ...grpc.CallOption) (*LoadSnapshotResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(LoadSnapshotResponse)
	err := c.cc.Invoke(ctx, JQL_LoadSnapshot_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// JQLServer is the server API for JQL service.
// All implementations must embed UnimplementedJQLServer
// for forward compatibility.
type JQLServer interface {
	ListTables(context.Context, *ListTablesRequest) (*ListTablesResponse, error)
	ListRows(context.Context, *ListRowsRequest) (*ListRowsResponse, error)
	GetRow(context.Context, *GetRowRequest) (*GetRowResponse, error)
	WriteRow(context.Context, *WriteRowRequest) (*WriteRowResponse, error)
	DeleteRow(context.Context, *DeleteRowRequest) (*DeleteRowResponse, error)
	IncrementEntry(context.Context, *IncrementEntryRequest) (*IncrementEntryResponse, error)
	Persist(context.Context, *PersistRequest) (*PersistResponse, error)
	GetSnapshot(context.Context, *GetSnapshotRequest) (*GetSnapshotResponse, error)
	LoadSnapshot(context.Context, *LoadSnapshotRequest) (*LoadSnapshotResponse, error)
	mustEmbedUnimplementedJQLServer()
}

// UnimplementedJQLServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedJQLServer struct{}

func (UnimplementedJQLServer) ListTables(context.Context, *ListTablesRequest) (*ListTablesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListTables not implemented")
}
func (UnimplementedJQLServer) ListRows(context.Context, *ListRowsRequest) (*ListRowsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListRows not implemented")
}
func (UnimplementedJQLServer) GetRow(context.Context, *GetRowRequest) (*GetRowResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRow not implemented")
}
func (UnimplementedJQLServer) WriteRow(context.Context, *WriteRowRequest) (*WriteRowResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method WriteRow not implemented")
}
func (UnimplementedJQLServer) DeleteRow(context.Context, *DeleteRowRequest) (*DeleteRowResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteRow not implemented")
}
func (UnimplementedJQLServer) IncrementEntry(context.Context, *IncrementEntryRequest) (*IncrementEntryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method IncrementEntry not implemented")
}
func (UnimplementedJQLServer) Persist(context.Context, *PersistRequest) (*PersistResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Persist not implemented")
}
func (UnimplementedJQLServer) GetSnapshot(context.Context, *GetSnapshotRequest) (*GetSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSnapshot not implemented")
}
func (UnimplementedJQLServer) LoadSnapshot(context.Context, *LoadSnapshotRequest) (*LoadSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method LoadSnapshot not implemented")
}
func (UnimplementedJQLServer) mustEmbedUnimplementedJQLServer() {}
func (UnimplementedJQLServer) testEmbeddedByValue()             {}

// UnsafeJQLServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to JQLServer will
// result in compilation errors.
type UnsafeJQLServer interface {
	mustEmbedUnimplementedJQLServer()
}

func RegisterJQLServer(s grpc.ServiceRegistrar, srv JQLServer) {
	// If the following call pancis, it indicates UnimplementedJQLServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&JQL_ServiceDesc, srv)
}

func _JQL_ListTables_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListTablesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).ListTables(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_ListTables_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).ListTables(ctx, req.(*ListTablesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JQL_ListRows_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListRowsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).ListRows(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_ListRows_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).ListRows(ctx, req.(*ListRowsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JQL_GetRow_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRowRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).GetRow(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_GetRow_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).GetRow(ctx, req.(*GetRowRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JQL_WriteRow_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(WriteRowRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).WriteRow(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_WriteRow_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).WriteRow(ctx, req.(*WriteRowRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JQL_DeleteRow_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteRowRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).DeleteRow(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_DeleteRow_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).DeleteRow(ctx, req.(*DeleteRowRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JQL_IncrementEntry_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(IncrementEntryRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).IncrementEntry(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_IncrementEntry_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).IncrementEntry(ctx, req.(*IncrementEntryRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JQL_Persist_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PersistRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).Persist(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_Persist_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).Persist(ctx, req.(*PersistRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JQL_GetSnapshot_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetSnapshotRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).GetSnapshot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_GetSnapshot_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).GetSnapshot(ctx, req.(*GetSnapshotRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JQL_LoadSnapshot_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(LoadSnapshotRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JQLServer).LoadSnapshot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: JQL_LoadSnapshot_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JQLServer).LoadSnapshot(ctx, req.(*LoadSnapshotRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// JQL_ServiceDesc is the grpc.ServiceDesc for JQL service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var JQL_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "jql.JQL",
	HandlerType: (*JQLServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListTables",
			Handler:    _JQL_ListTables_Handler,
		},
		{
			MethodName: "ListRows",
			Handler:    _JQL_ListRows_Handler,
		},
		{
			MethodName: "GetRow",
			Handler:    _JQL_GetRow_Handler,
		},
		{
			MethodName: "WriteRow",
			Handler:    _JQL_WriteRow_Handler,
		},
		{
			MethodName: "DeleteRow",
			Handler:    _JQL_DeleteRow_Handler,
		},
		{
			MethodName: "IncrementEntry",
			Handler:    _JQL_IncrementEntry_Handler,
		},
		{
			MethodName: "Persist",
			Handler:    _JQL_Persist_Handler,
		},
		{
			MethodName: "GetSnapshot",
			Handler:    _JQL_GetSnapshot_Handler,
		},
		{
			MethodName: "LoadSnapshot",
			Handler:    _JQL_LoadSnapshot_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "jql/jql.proto",
}
