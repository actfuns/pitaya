package docgenerator

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func ProtoDescriptors(messageName string) ([]byte, error) {
	// 查找 message 类型
	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(messageName))
	if err != nil {
		return nil, err
	}

	fd := mt.Descriptor().ParentFile()
	fdp := protodesc.ToFileDescriptorProto(fd)

	return proto.Marshal(fdp)
}
