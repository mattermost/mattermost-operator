// Code generated by protoc-gen-go. DO NOT EDIT.
// source: google/ads/googleads/v1/errors/asset_error.proto

package errors // import "google.golang.org/genproto/googleapis/ads/googleads/v1/errors"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import _ "google.golang.org/genproto/googleapis/api/annotations"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// Enum describing possible asset errors.
type AssetErrorEnum_AssetError int32

const (
	// Enum unspecified.
	AssetErrorEnum_UNSPECIFIED AssetErrorEnum_AssetError = 0
	// The received error code is not known in this version.
	AssetErrorEnum_UNKNOWN AssetErrorEnum_AssetError = 1
	// The customer is not whitelisted for this asset type.
	AssetErrorEnum_CUSTOMER_NOT_WHITELISTED_FOR_ASSET_TYPE AssetErrorEnum_AssetError = 2
	// Assets are duplicated across operations.
	AssetErrorEnum_DUPLICATE_ASSET AssetErrorEnum_AssetError = 3
	// The asset name is duplicated, either across operations or with an
	// existing asset.
	AssetErrorEnum_DUPLICATE_ASSET_NAME AssetErrorEnum_AssetError = 4
	// The Asset.asset_data oneof is empty.
	AssetErrorEnum_ASSET_DATA_IS_MISSING AssetErrorEnum_AssetError = 5
)

var AssetErrorEnum_AssetError_name = map[int32]string{
	0: "UNSPECIFIED",
	1: "UNKNOWN",
	2: "CUSTOMER_NOT_WHITELISTED_FOR_ASSET_TYPE",
	3: "DUPLICATE_ASSET",
	4: "DUPLICATE_ASSET_NAME",
	5: "ASSET_DATA_IS_MISSING",
}
var AssetErrorEnum_AssetError_value = map[string]int32{
	"UNSPECIFIED": 0,
	"UNKNOWN":     1,
	"CUSTOMER_NOT_WHITELISTED_FOR_ASSET_TYPE": 2,
	"DUPLICATE_ASSET":                         3,
	"DUPLICATE_ASSET_NAME":                    4,
	"ASSET_DATA_IS_MISSING":                   5,
}

func (x AssetErrorEnum_AssetError) String() string {
	return proto.EnumName(AssetErrorEnum_AssetError_name, int32(x))
}
func (AssetErrorEnum_AssetError) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_asset_error_caa6e7fc75536520, []int{0, 0}
}

// Container for enum describing possible asset errors.
type AssetErrorEnum struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *AssetErrorEnum) Reset()         { *m = AssetErrorEnum{} }
func (m *AssetErrorEnum) String() string { return proto.CompactTextString(m) }
func (*AssetErrorEnum) ProtoMessage()    {}
func (*AssetErrorEnum) Descriptor() ([]byte, []int) {
	return fileDescriptor_asset_error_caa6e7fc75536520, []int{0}
}
func (m *AssetErrorEnum) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_AssetErrorEnum.Unmarshal(m, b)
}
func (m *AssetErrorEnum) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_AssetErrorEnum.Marshal(b, m, deterministic)
}
func (dst *AssetErrorEnum) XXX_Merge(src proto.Message) {
	xxx_messageInfo_AssetErrorEnum.Merge(dst, src)
}
func (m *AssetErrorEnum) XXX_Size() int {
	return xxx_messageInfo_AssetErrorEnum.Size(m)
}
func (m *AssetErrorEnum) XXX_DiscardUnknown() {
	xxx_messageInfo_AssetErrorEnum.DiscardUnknown(m)
}

var xxx_messageInfo_AssetErrorEnum proto.InternalMessageInfo

func init() {
	proto.RegisterType((*AssetErrorEnum)(nil), "google.ads.googleads.v1.errors.AssetErrorEnum")
	proto.RegisterEnum("google.ads.googleads.v1.errors.AssetErrorEnum_AssetError", AssetErrorEnum_AssetError_name, AssetErrorEnum_AssetError_value)
}

func init() {
	proto.RegisterFile("google/ads/googleads/v1/errors/asset_error.proto", fileDescriptor_asset_error_caa6e7fc75536520)
}

var fileDescriptor_asset_error_caa6e7fc75536520 = []byte{
	// 363 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x90, 0xcd, 0x6a, 0xe3, 0x30,
	0x14, 0x85, 0xc7, 0xce, 0xfc, 0x80, 0x02, 0x13, 0xe3, 0x99, 0x81, 0x99, 0x61, 0xc8, 0xc2, 0x9b,
	0x59, 0x14, 0xe4, 0x9a, 0xee, 0xd4, 0x95, 0x12, 0x2b, 0xa9, 0x68, 0x62, 0x9b, 0x48, 0x4e, 0x68,
	0x31, 0x08, 0xb7, 0x36, 0x26, 0x90, 0x58, 0xc1, 0x72, 0xf3, 0x3e, 0xed, 0xb2, 0xeb, 0x3e, 0x45,
	0x1f, 0x25, 0x4f, 0x51, 0x6c, 0x35, 0x09, 0x14, 0xda, 0x95, 0xce, 0xbd, 0xfa, 0xce, 0x95, 0xee,
	0x01, 0xa7, 0x85, 0x94, 0xc5, 0x2a, 0x77, 0xd3, 0x4c, 0xb9, 0x5a, 0x36, 0x6a, 0xeb, 0xb9, 0x79,
	0x55, 0xc9, 0x4a, 0xb9, 0xa9, 0x52, 0x79, 0x2d, 0xda, 0x02, 0x6e, 0x2a, 0x59, 0x4b, 0xbb, 0xaf,
	0x31, 0x98, 0x66, 0x0a, 0x1e, 0x1c, 0x70, 0xeb, 0x41, 0xed, 0xf8, 0xfb, 0x6f, 0x3f, 0x71, 0xb3,
	0x74, 0xd3, 0xb2, 0x94, 0x75, 0x5a, 0x2f, 0x65, 0xa9, 0xb4, 0xdb, 0x79, 0x32, 0xc0, 0x77, 0xdc,
	0xcc, 0x24, 0x0d, 0x4d, 0xca, 0xbb, 0xb5, 0x73, 0x6f, 0x00, 0x70, 0x6c, 0xd9, 0x3d, 0xd0, 0x8d,
	0x03, 0x16, 0x91, 0x21, 0x1d, 0x51, 0xe2, 0x5b, 0x9f, 0xec, 0x2e, 0xf8, 0x16, 0x07, 0x97, 0x41,
	0xb8, 0x08, 0x2c, 0xc3, 0x3e, 0x01, 0xff, 0x87, 0x31, 0xe3, 0xe1, 0x94, 0xcc, 0x44, 0x10, 0x72,
	0xb1, 0xb8, 0xa0, 0x9c, 0x4c, 0x28, 0xe3, 0xc4, 0x17, 0xa3, 0x70, 0x26, 0x30, 0x63, 0x84, 0x0b,
	0x7e, 0x15, 0x11, 0xcb, 0xb4, 0x7f, 0x80, 0x9e, 0x1f, 0x47, 0x13, 0x3a, 0xc4, 0x9c, 0xe8, 0x1b,
	0xab, 0x63, 0xff, 0x06, 0x3f, 0xdf, 0x34, 0x45, 0x80, 0xa7, 0xc4, 0xfa, 0x6c, 0xff, 0x01, 0xbf,
	0x74, 0xed, 0x63, 0x8e, 0x05, 0x65, 0x62, 0x4a, 0x19, 0xa3, 0xc1, 0xd8, 0xfa, 0x32, 0xd8, 0x19,
	0xc0, 0xb9, 0x95, 0x6b, 0xf8, 0xf1, 0xee, 0x83, 0xde, 0x71, 0x8f, 0xa8, 0x59, 0x37, 0x32, 0xae,
	0xfd, 0x57, 0x4b, 0x21, 0x57, 0x69, 0x59, 0x40, 0x59, 0x15, 0x6e, 0x91, 0x97, 0x6d, 0x18, 0xfb,
	0xc0, 0x37, 0x4b, 0xf5, 0x5e, 0xfe, 0xe7, 0xfa, 0x78, 0x30, 0x3b, 0x63, 0x8c, 0x1f, 0xcd, 0xfe,
	0x58, 0x0f, 0xc3, 0x99, 0x82, 0x5a, 0x36, 0x6a, 0xee, 0xc1, 0xf6, 0x49, 0xf5, 0xbc, 0x07, 0x12,
	0x9c, 0xa9, 0xe4, 0x00, 0x24, 0x73, 0x2f, 0xd1, 0xc0, 0xce, 0x74, 0x74, 0x17, 0x21, 0x9c, 0x29,
	0x84, 0x0e, 0x08, 0x42, 0x73, 0x0f, 0x21, 0x0d, 0xdd, 0x7c, 0x6d, 0x7f, 0x77, 0xf6, 0x12, 0x00,
	0x00, 0xff, 0xff, 0xad, 0xf5, 0xf8, 0x32, 0x1c, 0x02, 0x00, 0x00,
}
