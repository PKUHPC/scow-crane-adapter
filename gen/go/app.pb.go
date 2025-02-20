//*
// Copyright (c) 2022 Peking University and Peking University Institute for Computing and Digital Economy
// SCOW is licensed under Mulan PSL v2.
// You can use this software according to the terms and conditions of the Mulan PSL v2.
// You may obtain a copy of Mulan PSL v2 at:
//          http://license.coscl.org.cn/MulanPSL2
// THIS SOFTWARE IS PROVIDED ON AN "AS IS" BASIS, WITHOUT WARRANTIES OF ANY KIND,
// EITHER EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO NON-INFRINGEMENT,
// MERCHANTABILITY OR FIT FOR A PARTICULAR PURPOSE.
// See the Mulan PSL v2 for more details.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        (unknown)
// source: app.proto

package gen

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type GetAppConnectionInfoRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	JobId         uint32                 `protobuf:"varint,1,opt,name=job_id,json=jobId,proto3" json:"job_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetAppConnectionInfoRequest) Reset() {
	*x = GetAppConnectionInfoRequest{}
	mi := &file_app_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetAppConnectionInfoRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetAppConnectionInfoRequest) ProtoMessage() {}

func (x *GetAppConnectionInfoRequest) ProtoReflect() protoreflect.Message {
	mi := &file_app_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetAppConnectionInfoRequest.ProtoReflect.Descriptor instead.
func (*GetAppConnectionInfoRequest) Descriptor() ([]byte, []int) {
	return file_app_proto_rawDescGZIP(), []int{0}
}

func (x *GetAppConnectionInfoRequest) GetJobId() uint32 {
	if x != nil {
		return x.JobId
	}
	return 0
}

type GetAppConnectionInfoResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Response:
	//
	//	*GetAppConnectionInfoResponse_UseJobScriptGenerated_
	//	*GetAppConnectionInfoResponse_AppConnectionInfo_
	Response      isGetAppConnectionInfoResponse_Response `protobuf_oneof:"response"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetAppConnectionInfoResponse) Reset() {
	*x = GetAppConnectionInfoResponse{}
	mi := &file_app_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetAppConnectionInfoResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetAppConnectionInfoResponse) ProtoMessage() {}

func (x *GetAppConnectionInfoResponse) ProtoReflect() protoreflect.Message {
	mi := &file_app_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetAppConnectionInfoResponse.ProtoReflect.Descriptor instead.
func (*GetAppConnectionInfoResponse) Descriptor() ([]byte, []int) {
	return file_app_proto_rawDescGZIP(), []int{1}
}

func (x *GetAppConnectionInfoResponse) GetResponse() isGetAppConnectionInfoResponse_Response {
	if x != nil {
		return x.Response
	}
	return nil
}

func (x *GetAppConnectionInfoResponse) GetUseJobScriptGenerated() *GetAppConnectionInfoResponse_UseJobScriptGenerated {
	if x != nil {
		if x, ok := x.Response.(*GetAppConnectionInfoResponse_UseJobScriptGenerated_); ok {
			return x.UseJobScriptGenerated
		}
	}
	return nil
}

func (x *GetAppConnectionInfoResponse) GetAppConnectionInfo() *GetAppConnectionInfoResponse_AppConnectionInfo {
	if x != nil {
		if x, ok := x.Response.(*GetAppConnectionInfoResponse_AppConnectionInfo_); ok {
			return x.AppConnectionInfo
		}
	}
	return nil
}

type isGetAppConnectionInfoResponse_Response interface {
	isGetAppConnectionInfoResponse_Response()
}

type GetAppConnectionInfoResponse_UseJobScriptGenerated_ struct {
	UseJobScriptGenerated *GetAppConnectionInfoResponse_UseJobScriptGenerated `protobuf:"bytes,1,opt,name=use_job_script_generated,json=useJobScriptGenerated,proto3,oneof"`
}

type GetAppConnectionInfoResponse_AppConnectionInfo_ struct {
	AppConnectionInfo *GetAppConnectionInfoResponse_AppConnectionInfo `protobuf:"bytes,2,opt,name=app_connection_info,json=appConnectionInfo,proto3,oneof"`
}

func (*GetAppConnectionInfoResponse_UseJobScriptGenerated_) isGetAppConnectionInfoResponse_Response() {
}

func (*GetAppConnectionInfoResponse_AppConnectionInfo_) isGetAppConnectionInfoResponse_Response() {}

type GetAppConnectionInfoResponse_UseJobScriptGenerated struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetAppConnectionInfoResponse_UseJobScriptGenerated) Reset() {
	*x = GetAppConnectionInfoResponse_UseJobScriptGenerated{}
	mi := &file_app_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetAppConnectionInfoResponse_UseJobScriptGenerated) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetAppConnectionInfoResponse_UseJobScriptGenerated) ProtoMessage() {}

func (x *GetAppConnectionInfoResponse_UseJobScriptGenerated) ProtoReflect() protoreflect.Message {
	mi := &file_app_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetAppConnectionInfoResponse_UseJobScriptGenerated.ProtoReflect.Descriptor instead.
func (*GetAppConnectionInfoResponse_UseJobScriptGenerated) Descriptor() ([]byte, []int) {
	return file_app_proto_rawDescGZIP(), []int{1, 0}
}

type GetAppConnectionInfoResponse_AppConnectionInfo struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Host          string                 `protobuf:"bytes,1,opt,name=host,proto3" json:"host,omitempty"`
	Port          uint32                 `protobuf:"varint,2,opt,name=port,proto3" json:"port,omitempty"`
	Password      string                 `protobuf:"bytes,3,opt,name=password,proto3" json:"password,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetAppConnectionInfoResponse_AppConnectionInfo) Reset() {
	*x = GetAppConnectionInfoResponse_AppConnectionInfo{}
	mi := &file_app_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetAppConnectionInfoResponse_AppConnectionInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetAppConnectionInfoResponse_AppConnectionInfo) ProtoMessage() {}

func (x *GetAppConnectionInfoResponse_AppConnectionInfo) ProtoReflect() protoreflect.Message {
	mi := &file_app_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetAppConnectionInfoResponse_AppConnectionInfo.ProtoReflect.Descriptor instead.
func (*GetAppConnectionInfoResponse_AppConnectionInfo) Descriptor() ([]byte, []int) {
	return file_app_proto_rawDescGZIP(), []int{1, 1}
}

func (x *GetAppConnectionInfoResponse_AppConnectionInfo) GetHost() string {
	if x != nil {
		return x.Host
	}
	return ""
}

func (x *GetAppConnectionInfoResponse_AppConnectionInfo) GetPort() uint32 {
	if x != nil {
		return x.Port
	}
	return 0
}

func (x *GetAppConnectionInfoResponse_AppConnectionInfo) GetPassword() string {
	if x != nil {
		return x.Password
	}
	return ""
}

var File_app_proto protoreflect.FileDescriptor

var file_app_proto_rawDesc = string([]byte{
	0x0a, 0x09, 0x61, 0x70, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x16, 0x73, 0x63, 0x6f,
	0x77, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x5f, 0x61, 0x64, 0x61, 0x70,
	0x74, 0x65, 0x72, 0x22, 0x34, 0x0a, 0x1b, 0x47, 0x65, 0x74, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e,
	0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x15, 0x0a, 0x06, 0x6a, 0x6f, 0x62, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0d, 0x52, 0x05, 0x6a, 0x6f, 0x62, 0x49, 0x64, 0x22, 0x9e, 0x03, 0x0a, 0x1c, 0x47, 0x65,
	0x74, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e,
	0x66, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x85, 0x01, 0x0a, 0x18, 0x75,
	0x73, 0x65, 0x5f, 0x6a, 0x6f, 0x62, 0x5f, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x5f, 0x67, 0x65,
	0x6e, 0x65, 0x72, 0x61, 0x74, 0x65, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x4a, 0x2e,
	0x73, 0x63, 0x6f, 0x77, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x5f, 0x61,
	0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x2e, 0x47, 0x65, 0x74, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e,
	0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x2e, 0x55, 0x73, 0x65, 0x4a, 0x6f, 0x62, 0x53, 0x63, 0x72, 0x69, 0x70, 0x74,
	0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74, 0x65, 0x64, 0x48, 0x00, 0x52, 0x15, 0x75, 0x73, 0x65,
	0x4a, 0x6f, 0x62, 0x53, 0x63, 0x72, 0x69, 0x70, 0x74, 0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x74,
	0x65, 0x64, 0x12, 0x78, 0x0a, 0x13, 0x61, 0x70, 0x70, 0x5f, 0x63, 0x6f, 0x6e, 0x6e, 0x65, 0x63,
	0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x6e, 0x66, 0x6f, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x46, 0x2e, 0x73, 0x63, 0x6f, 0x77, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72,
	0x5f, 0x61, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x2e, 0x47, 0x65, 0x74, 0x41, 0x70, 0x70, 0x43,
	0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x2e, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x66, 0x6f, 0x48, 0x00, 0x52, 0x11, 0x61, 0x70, 0x70, 0x43, 0x6f,
	0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x66, 0x6f, 0x1a, 0x17, 0x0a, 0x15,
	0x55, 0x73, 0x65, 0x4a, 0x6f, 0x62, 0x53, 0x63, 0x72, 0x69, 0x70, 0x74, 0x47, 0x65, 0x6e, 0x65,
	0x72, 0x61, 0x74, 0x65, 0x64, 0x1a, 0x57, 0x0a, 0x11, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e, 0x6e,
	0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x66, 0x6f, 0x12, 0x12, 0x0a, 0x04, 0x68, 0x6f,
	0x73, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x12, 0x12,
	0x0a, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x70, 0x6f,
	0x72, 0x74, 0x12, 0x1a, 0x0a, 0x08, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x42, 0x0a,
	0x0a, 0x08, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x32, 0x90, 0x01, 0x0a, 0x0a, 0x41,
	0x70, 0x70, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x81, 0x01, 0x0a, 0x14, 0x47, 0x65,
	0x74, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e,
	0x66, 0x6f, 0x12, 0x33, 0x2e, 0x73, 0x63, 0x6f, 0x77, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75,
	0x6c, 0x65, 0x72, 0x5f, 0x61, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x2e, 0x47, 0x65, 0x74, 0x41,
	0x70, 0x70, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x66, 0x6f,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x34, 0x2e, 0x73, 0x63, 0x6f, 0x77, 0x2e, 0x73,
	0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x5f, 0x61, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72,
	0x2e, 0x47, 0x65, 0x74, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e, 0x6e, 0x65, 0x63, 0x74, 0x69, 0x6f,
	0x6e, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0xb3, 0x01,
	0x0a, 0x1a, 0x63, 0x6f, 0x6d, 0x2e, 0x73, 0x63, 0x6f, 0x77, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64,
	0x75, 0x6c, 0x65, 0x72, 0x5f, 0x61, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x42, 0x08, 0x41, 0x70,
	0x70, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x16, 0x73, 0x63, 0x6f, 0x77, 0x2d, 0x63,
	0x72, 0x61, 0x6e, 0x65, 0x2d, 0x61, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x2f, 0x67, 0x65, 0x6e,
	0xa2, 0x02, 0x03, 0x53, 0x53, 0x58, 0xaa, 0x02, 0x15, 0x53, 0x63, 0x6f, 0x77, 0x2e, 0x53, 0x63,
	0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x41, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0xca, 0x02,
	0x15, 0x53, 0x63, 0x6f, 0x77, 0x5c, 0x53, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x41,
	0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0xe2, 0x02, 0x21, 0x53, 0x63, 0x6f, 0x77, 0x5c, 0x53, 0x63,
	0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x41, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x5c, 0x47,
	0x50, 0x42, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea, 0x02, 0x16, 0x53, 0x63, 0x6f,
	0x77, 0x3a, 0x3a, 0x53, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x41, 0x64, 0x61, 0x70,
	0x74, 0x65, 0x72, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_app_proto_rawDescOnce sync.Once
	file_app_proto_rawDescData []byte
)

func file_app_proto_rawDescGZIP() []byte {
	file_app_proto_rawDescOnce.Do(func() {
		file_app_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_app_proto_rawDesc), len(file_app_proto_rawDesc)))
	})
	return file_app_proto_rawDescData
}

var file_app_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_app_proto_goTypes = []any{
	(*GetAppConnectionInfoRequest)(nil),                        // 0: scow.scheduler_adapter.GetAppConnectionInfoRequest
	(*GetAppConnectionInfoResponse)(nil),                       // 1: scow.scheduler_adapter.GetAppConnectionInfoResponse
	(*GetAppConnectionInfoResponse_UseJobScriptGenerated)(nil), // 2: scow.scheduler_adapter.GetAppConnectionInfoResponse.UseJobScriptGenerated
	(*GetAppConnectionInfoResponse_AppConnectionInfo)(nil),     // 3: scow.scheduler_adapter.GetAppConnectionInfoResponse.AppConnectionInfo
}
var file_app_proto_depIdxs = []int32{
	2, // 0: scow.scheduler_adapter.GetAppConnectionInfoResponse.use_job_script_generated:type_name -> scow.scheduler_adapter.GetAppConnectionInfoResponse.UseJobScriptGenerated
	3, // 1: scow.scheduler_adapter.GetAppConnectionInfoResponse.app_connection_info:type_name -> scow.scheduler_adapter.GetAppConnectionInfoResponse.AppConnectionInfo
	0, // 2: scow.scheduler_adapter.AppService.GetAppConnectionInfo:input_type -> scow.scheduler_adapter.GetAppConnectionInfoRequest
	1, // 3: scow.scheduler_adapter.AppService.GetAppConnectionInfo:output_type -> scow.scheduler_adapter.GetAppConnectionInfoResponse
	3, // [3:4] is the sub-list for method output_type
	2, // [2:3] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_app_proto_init() }
func file_app_proto_init() {
	if File_app_proto != nil {
		return
	}
	file_app_proto_msgTypes[1].OneofWrappers = []any{
		(*GetAppConnectionInfoResponse_UseJobScriptGenerated_)(nil),
		(*GetAppConnectionInfoResponse_AppConnectionInfo_)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_app_proto_rawDesc), len(file_app_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_app_proto_goTypes,
		DependencyIndexes: file_app_proto_depIdxs,
		MessageInfos:      file_app_proto_msgTypes,
	}.Build()
	File_app_proto = out.File
	file_app_proto_goTypes = nil
	file_app_proto_depIdxs = nil
}
