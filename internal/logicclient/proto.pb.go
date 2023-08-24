// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.27.1
// 	protoc        v3.18.1
// source: internal/logicclient/proto.proto

package logicclient

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type AuthReq struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Uid        string `protobuf:"bytes,1,opt,name=uid,proto3" json:"uid,omitempty"`
	Token      string `protobuf:"bytes,2,opt,name=token,proto3" json:"token,omitempty"`
	DeviceFlag uint32 `protobuf:"varint,3,opt,name=deviceFlag,proto3" json:"deviceFlag,omitempty"`
}

func (x *AuthReq) Reset() {
	*x = AuthReq{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_logicclient_proto_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AuthReq) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AuthReq) ProtoMessage() {}

func (x *AuthReq) ProtoReflect() protoreflect.Message {
	mi := &file_internal_logicclient_proto_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AuthReq.ProtoReflect.Descriptor instead.
func (*AuthReq) Descriptor() ([]byte, []int) {
	return file_internal_logicclient_proto_proto_rawDescGZIP(), []int{0}
}

func (x *AuthReq) GetUid() string {
	if x != nil {
		return x.Uid
	}
	return ""
}

func (x *AuthReq) GetToken() string {
	if x != nil {
		return x.Token
	}
	return ""
}

func (x *AuthReq) GetDeviceFlag() uint32 {
	if x != nil {
		return x.DeviceFlag
	}
	return 0
}

type AuthResp struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	DeviceLevel uint32 `protobuf:"varint,1,opt,name=deviceLevel,proto3" json:"deviceLevel,omitempty"`
}

func (x *AuthResp) Reset() {
	*x = AuthResp{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_logicclient_proto_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AuthResp) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AuthResp) ProtoMessage() {}

func (x *AuthResp) ProtoReflect() protoreflect.Message {
	mi := &file_internal_logicclient_proto_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AuthResp.ProtoReflect.Descriptor instead.
func (*AuthResp) Descriptor() ([]byte, []int) {
	return file_internal_logicclient_proto_proto_rawDescGZIP(), []int{1}
}

func (x *AuthResp) GetDeviceLevel() uint32 {
	if x != nil {
		return x.DeviceLevel
	}
	return 0
}

var File_internal_logicclient_proto_proto protoreflect.FileDescriptor

var file_internal_logicclient_proto_proto_rawDesc = []byte{
	0x0a, 0x20, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x6c, 0x6f, 0x67, 0x69, 0x63,
	0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x12, 0x0b, 0x6c, 0x6f, 0x67, 0x69, 0x63, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x22,
	0x51, 0x0a, 0x07, 0x41, 0x75, 0x74, 0x68, 0x52, 0x65, 0x71, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x69,
	0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x75, 0x69, 0x64, 0x12, 0x14, 0x0a, 0x05,
	0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x74, 0x6f, 0x6b,
	0x65, 0x6e, 0x12, 0x1e, 0x0a, 0x0a, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x46, 0x6c, 0x61, 0x67,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0a, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x46, 0x6c,
	0x61, 0x67, 0x22, 0x2c, 0x0a, 0x08, 0x41, 0x75, 0x74, 0x68, 0x52, 0x65, 0x73, 0x70, 0x12, 0x20,
	0x0a, 0x0b, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0d, 0x52, 0x0b, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x4c, 0x65, 0x76, 0x65, 0x6c,
	0x42, 0x10, 0x5a, 0x0e, 0x2e, 0x2f, 0x3b, 0x6c, 0x6f, 0x67, 0x69, 0x63, 0x63, 0x6c, 0x69, 0x65,
	0x6e, 0x74, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_internal_logicclient_proto_proto_rawDescOnce sync.Once
	file_internal_logicclient_proto_proto_rawDescData = file_internal_logicclient_proto_proto_rawDesc
)

func file_internal_logicclient_proto_proto_rawDescGZIP() []byte {
	file_internal_logicclient_proto_proto_rawDescOnce.Do(func() {
		file_internal_logicclient_proto_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_logicclient_proto_proto_rawDescData)
	})
	return file_internal_logicclient_proto_proto_rawDescData
}

var file_internal_logicclient_proto_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_internal_logicclient_proto_proto_goTypes = []interface{}{
	(*AuthReq)(nil),  // 0: logicclient.AuthReq
	(*AuthResp)(nil), // 1: logicclient.AuthResp
}
var file_internal_logicclient_proto_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_internal_logicclient_proto_proto_init() }
func file_internal_logicclient_proto_proto_init() {
	if File_internal_logicclient_proto_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_internal_logicclient_proto_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AuthReq); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_logicclient_proto_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AuthResp); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_internal_logicclient_proto_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_internal_logicclient_proto_proto_goTypes,
		DependencyIndexes: file_internal_logicclient_proto_proto_depIdxs,
		MessageInfos:      file_internal_logicclient_proto_proto_msgTypes,
	}.Build()
	File_internal_logicclient_proto_proto = out.File
	file_internal_logicclient_proto_proto_rawDesc = nil
	file_internal_logicclient_proto_proto_goTypes = nil
	file_internal_logicclient_proto_proto_depIdxs = nil
}
