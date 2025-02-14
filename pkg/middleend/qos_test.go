// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Intel Corporation

// Package middleend implements the MiddleEnd APIs (service) of the storage Server
package middleend

import (
	"fmt"
	"net"
	"testing"

	"github.com/opiproject/gospdk/spdk"
	pb "github.com/opiproject/opi-api/storage/v1alpha1/gen/go"
	"github.com/opiproject/opi-spdk-bridge/pkg/server"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

var (
	testQosVolumeID   = "qos-volume-42"
	testQosVolumeName = server.ResourceIDToVolumeName(testQosVolumeID)
	testQosVolume     = &pb.QosVolume{
		VolumeNameRef: "volume-42",
		Limits: &pb.Limits{
			Max: &pb.QosLimit{RwBandwidthMbs: 1},
		},
	}
)

type stubJSONRRPC struct {
	params []any
}

func (s *stubJSONRRPC) GetID() uint64 {
	return 0
}

func (s *stubJSONRRPC) StartUnixListener() net.Listener {
	return nil
}

func (s *stubJSONRRPC) GetVersion() string {
	return ""
}

func (s *stubJSONRRPC) Call(_ string, param interface{}, _ interface{}) error {
	s.params = append(s.params, param)
	return nil
}

func TestMiddleEnd_CreateQosVolume(t *testing.T) {
	t.Cleanup(checkGlobalTestProtoObjectsNotChanged(t, t.Name()))
	tests := map[string]struct {
		id          string
		in          *pb.QosVolume
		out         *pb.QosVolume
		spdk        []string
		errCode     codes.Code
		errMsg      string
		existBefore bool
		existAfter  bool
	}{
		"min_limit is not supported": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Min: &pb.QosLimit{
						RdIopsKiops: 100000,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume min_limit is not supported",
			existBefore: false,
			existAfter:  false,
		},
		"max_limit rd_iops_kiops is not supported": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RdIopsKiops: 100000,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit rd_iops_kiops is not supported",
			existBefore: false,
			existAfter:  false,
		},
		"max_limit wr_iops_kiops is not supported": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						WrIopsKiops: 100000,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit wr_iops_kiops is not supported",
			existBefore: false,
			existAfter:  false,
		},
		"max_limit rw_iops_kiops is negative": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RwIopsKiops: -1,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit rw_iops_kiops cannot be negative",
			existBefore: false,
			existAfter:  false,
		},
		"max_limit rd_bandwidth_kiops is negative": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RdBandwidthMbs: -1,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit rd_bandwidth_mbs cannot be negative",
			existBefore: false,
			existAfter:  false,
		},
		"max_limit wr_bandwidth_kiops is negative": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						WrBandwidthMbs: -1,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit wr_bandwidth_mbs cannot be negative",
			existBefore: false,
			existAfter:  false,
		},
		"max_limit rw_bandwidth_kiops is negative": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RwBandwidthMbs: -1,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit rw_bandwidth_mbs cannot be negative",
			existBefore: false,
			existAfter:  false,
		},
		"max_limit with all zero limits": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit should set limit",
			existBefore: false,
			existAfter:  false,
		},
		"qos_volume name is ignored": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				Name:          server.ResourceIDToVolumeName("ignored-id"),
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{RwBandwidthMbs: 1},
				},
			},
			out:         testQosVolume,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":true}`},
			errCode:     codes.OK,
			errMsg:      "",
			existBefore: false,
			existAfter:  true,
		},
		"volume_id is empty": {
			id: testQosVolumeID,
			in: &pb.QosVolume{
				VolumeNameRef: "",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{RwBandwidthMbs: 1},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.Unknown,
			errMsg:      "missing required field: qos_volume.volume_name_ref",
			existBefore: false,
			existAfter:  false,
		},
		"qos volume already exists": {
			id:          testQosVolumeID,
			in:          testQosVolume,
			out:         testQosVolume,
			spdk:        []string{},
			errCode:     codes.OK,
			errMsg:      "",
			existBefore: true,
			existAfter:  true,
		},
		"SPDK call failed": {
			id:          testQosVolumeID,
			in:          testQosVolume,
			out:         nil,
			spdk:        []string{`{"id":%d,"error":{"code":1,"message":"some internal error"},"result":true}`},
			errCode:     status.Convert(spdk.ErrFailedSpdkCall).Code(),
			errMsg:      status.Convert(spdk.ErrFailedSpdkCall).Message(),
			existBefore: false,
			existAfter:  false,
		},
		"SPDK call result false": {
			id:          testQosVolumeID,
			in:          testQosVolume,
			out:         nil,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":false}`},
			errCode:     status.Convert(spdk.ErrUnexpectedSpdkCallResult).Code(),
			errMsg:      status.Convert(spdk.ErrUnexpectedSpdkCallResult).Message(),
			existBefore: false,
			existAfter:  false,
		},
		"successful creation": {
			id:          testQosVolumeID,
			in:          testQosVolume,
			out:         testQosVolume,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":true}`},
			errCode:     codes.OK,
			errMsg:      "",
			existBefore: false,
			existAfter:  true,
		},
		"no required field": {
			id:          testQosVolumeID,
			in:          nil,
			out:         nil,
			spdk:        []string{},
			errCode:     codes.Unknown,
			errMsg:      "missing required field: qos_volume",
			existBefore: false,
			existAfter:  false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			testEnv := createTestEnvironment(tt.spdk)
			defer testEnv.Close()

			if tt.existBefore {
				testEnv.opiSpdkServer.volumes.qosVolumes[testQosVolumeName] = server.ProtoClone(tt.out)
				testEnv.opiSpdkServer.volumes.qosVolumes[testQosVolumeName].Name = testQosVolumeName
			}
			if tt.out != nil {
				tt.out = server.ProtoClone(tt.out)
				tt.out.Name = testQosVolumeName
			}

			request := &pb.CreateQosVolumeRequest{QosVolume: tt.in, QosVolumeId: tt.id}
			response, err := testEnv.client.CreateQosVolume(testEnv.ctx, request)

			if !proto.Equal(response, tt.out) {
				t.Error("response: expected", tt.out, "received", response)
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}

			vol, ok := testEnv.opiSpdkServer.volumes.qosVolumes[testQosVolumeName]
			if tt.existAfter != ok {
				t.Error("expect QoS volume exist", tt.existAfter, "received", ok)
			}
			if tt.existAfter && !proto.Equal(tt.out, vol) {
				t.Error("expect QoS volume ", vol, "is equal to", tt.in)
			}
		})
	}

	t.Run("valid values are sent to SPDK", func(t *testing.T) {
		testEnv := createTestEnvironment([]string{})
		defer testEnv.Close()
		stubRPC := &stubJSONRRPC{}
		testEnv.opiSpdkServer.rpc = stubRPC

		_, _ = testEnv.client.CreateQosVolume(testEnv.ctx, &pb.CreateQosVolumeRequest{
			QosVolumeId: testQosVolumeID,
			QosVolume: &pb.QosVolume{
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RwIopsKiops:    1,
						RdBandwidthMbs: 2,
						WrBandwidthMbs: 3,
						RwBandwidthMbs: 4,
					},
				},
			},
		})
		if len(stubRPC.params) != 1 {
			t.Fatalf("Expect only one call to SPDK, received %v", stubRPC.params)
		}
		qosParams := stubRPC.params[0].(*spdk.BdevQoSParams)
		expectedParams := spdk.BdevQoSParams{
			Name:           "volume-42",
			RwIosPerSec:    1000,
			RMbytesPerSec:  2,
			WMbytesPerSec:  3,
			RwMbytesPerSec: 4,
		}
		if *qosParams != expectedParams {
			t.Errorf("Expected qos params to be sent: %v, received %v", expectedParams, *qosParams)
		}
	})
}

func TestMiddleEnd_DeleteQosVolume(t *testing.T) {
	t.Cleanup(checkGlobalTestProtoObjectsNotChanged(t, t.Name()))
	tests := map[string]struct {
		in          string
		spdk        []string
		errCode     codes.Code
		errMsg      string
		start       bool
		existBefore bool
		existAfter  bool
		missing     bool
	}{
		"qos volume does not exist": {
			in:          testQosVolumeID,
			spdk:        []string{},
			errCode:     codes.NotFound,
			errMsg:      fmt.Sprintf("unable to find key %s", server.ResourceIDToVolumeName(testQosVolumeID)),
			existBefore: false,
			existAfter:  false,
			missing:     false,
		},
		"qos volume does not exist, with allow_missing": {
			in:          testQosVolumeID,
			spdk:        []string{},
			errCode:     codes.OK,
			errMsg:      "",
			existBefore: false,
			existAfter:  false,
			missing:     true,
		},
		"SPDK call failed": {
			in:          testQosVolumeID,
			spdk:        []string{`{"id":%d,"error":{"code":1,"message":"some internal error"},"result":true}`},
			errCode:     status.Convert(spdk.ErrFailedSpdkCall).Code(),
			errMsg:      status.Convert(spdk.ErrFailedSpdkCall).Message(),
			existBefore: true,
			existAfter:  true,
			missing:     false,
		},
		"SPDK call result false": {
			in:          testQosVolumeID,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":false}`},
			errCode:     status.Convert(spdk.ErrUnexpectedSpdkCallResult).Code(),
			errMsg:      status.Convert(spdk.ErrUnexpectedSpdkCallResult).Message(),
			existBefore: true,
			existAfter:  true,
			missing:     false,
		},
		"successful deletion": {
			in:          testQosVolumeID,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":true}`},
			errCode:     codes.OK,
			errMsg:      "",
			existBefore: true,
			existAfter:  false,
			missing:     false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			testEnv := createTestEnvironment(tt.spdk)
			defer testEnv.Close()

			fname1 := server.ResourceIDToVolumeName(tt.in)

			request := &pb.DeleteQosVolumeRequest{Name: fname1}
			if tt.existBefore {
				testEnv.opiSpdkServer.volumes.qosVolumes[testQosVolumeName] = testQosVolume
			}
			if tt.missing {
				request.AllowMissing = true
			}

			_, err := testEnv.client.DeleteQosVolume(testEnv.ctx, request)

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}

			_, ok := testEnv.opiSpdkServer.volumes.qosVolumes[fname1]
			if tt.existAfter != ok {
				t.Error("expect QoS volume exist", tt.existAfter, "received", ok)
			}
		})
	}
}

func TestMiddleEnd_UpdateQosVolume(t *testing.T) {
	originalQosVolume := &pb.QosVolume{
		Name:          testQosVolumeName,
		VolumeNameRef: testQosVolume.VolumeNameRef,
		Limits: &pb.Limits{
			Max: &pb.QosLimit{RdBandwidthMbs: 1221},
		},
	}
	t.Cleanup(server.CheckTestProtoObjectsNotChanged(originalQosVolume)(t, t.Name()))
	t.Cleanup(checkGlobalTestProtoObjectsNotChanged(t, t.Name()))

	tests := map[string]struct {
		mask        *fieldmaskpb.FieldMask
		in          *pb.QosVolume
		out         *pb.QosVolume
		spdk        []string
		errCode     codes.Code
		errMsg      string
		existBefore bool
		missing     bool
	}{
		// "invalid fieldmask": {
		// 	mask: &fieldmaskpb.FieldMask{Paths: []string{"*", "author"}},
		// 	in: &pb.QosVolume{
		// 		Name:     testQosVolumeName,
		// 		VolumeNameRef: "volume-42",
		// 		MinLimit: &pb.QosLimit{
		// 			RdIopsKiops: 100000,
		// 		},
		// 	},
		// 	out:         nil,
		// 	spdk:        []string{},
		// 	errCode:     codes.Unknown,
		// 	errMsg:      fmt.Sprintf("invalid field path: %s", "'*' must not be used with other paths"),
		// 	existBefore: true,
		//	missing:	 false,
		// },
		"min_limit is not supported": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Min: &pb.QosLimit{
						RdIopsKiops: 100000,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume min_limit is not supported",
			existBefore: true,
			missing:     false,
		},
		"max_limit rd_iops_kiops is not supported": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RdIopsKiops: 100000,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit rd_iops_kiops is not supported",
			existBefore: true,
			missing:     false,
		},
		"max_limit wr_iops_kiops is not supported": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						WrIopsKiops: 100000,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit wr_iops_kiops is not supported",
			existBefore: true,
			missing:     false,
		},
		"max_limit rw_iops_kiops is negative": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RwIopsKiops: -1,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit rw_iops_kiops cannot be negative",
			existBefore: true,
			missing:     false,
		},
		"max_limit rd_bandwidth_kiops is negative": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RdBandwidthMbs: -1,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit rd_bandwidth_mbs cannot be negative",
			existBefore: true,
			missing:     false,
		},
		"max_limit wr_bandwidth_kiops is negative": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						WrBandwidthMbs: -1,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit wr_bandwidth_mbs cannot be negative",
			existBefore: true,
			missing:     false,
		},
		"max_limit rw_bandwidth_kiops is negative": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{
						RwBandwidthMbs: -1,
					},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit rw_bandwidth_mbs cannot be negative",
			existBefore: true,
			missing:     false,
		},
		"max_limit with all zero limits": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume max_limit should set limit",
			existBefore: true,
			missing:     false,
		},
		"qos_volume_id is empty": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          "",
				VolumeNameRef: "volume-42",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{RwBandwidthMbs: 1},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      "QoS volume name cannot be empty",
			existBefore: true,
			missing:     false,
		},
		"volume_id is empty": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{RwBandwidthMbs: 1},
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.Unknown,
			errMsg:      "missing required field: qos_volume.volume_name_ref",
			existBefore: true,
			missing:     false,
		},
		"qos volume does not exist": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: testQosVolume.VolumeNameRef,
				Limits: &pb.Limits{
					Max: testQosVolume.Limits.Max,
				},
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.NotFound,
			errMsg:      fmt.Sprintf("unable to find key %s", testQosVolumeName),
			existBefore: false,
			missing:     false,
		},
		"change underlying volume": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: "new-underlying-volume-id",
				Limits: &pb.Limits{
					Max: &pb.QosLimit{RdBandwidthMbs: 1},
				},
			},
			out:     nil,
			spdk:    []string{},
			errCode: codes.InvalidArgument,
			errMsg: fmt.Sprintf("Change of underlying volume %v to a new one %v is forbidden",
				originalQosVolume.VolumeNameRef, "new-underlying-volume-id"),
			existBefore: true,
			missing:     false,
		},
		"SPDK call failed": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: testQosVolume.VolumeNameRef,
				Limits: &pb.Limits{
					Max: testQosVolume.Limits.Max,
				},
			},
			out:         nil,
			spdk:        []string{`{"id":%d,"error":{"code":1,"message":"some internal error"},"result":true}`},
			errCode:     status.Convert(spdk.ErrFailedSpdkCall).Code(),
			errMsg:      status.Convert(spdk.ErrFailedSpdkCall).Message(),
			existBefore: true,
			missing:     false,
		},
		"SPDK call result false": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: testQosVolume.VolumeNameRef,
				Limits: &pb.Limits{
					Max: testQosVolume.Limits.Max,
				},
			},
			out:         nil,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":false}`},
			errCode:     status.Convert(spdk.ErrUnexpectedSpdkCallResult).Code(),
			errMsg:      status.Convert(spdk.ErrUnexpectedSpdkCallResult).Message(),
			existBefore: true,
			missing:     false,
		},
		"successful update": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: testQosVolume.VolumeNameRef,
				Limits: &pb.Limits{
					Max: &pb.QosLimit{RdBandwidthMbs: 2},
				},
			},
			out: &pb.QosVolume{
				Name:          testQosVolumeName,
				VolumeNameRef: testQosVolume.VolumeNameRef,
				Limits: &pb.Limits{
					Max: &pb.QosLimit{RdBandwidthMbs: 2},
				},
			},
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":true}`},
			errCode:     codes.OK,
			errMsg:      "",
			existBefore: true,
			missing:     false,
		},
		"update with the same limit values": {
			mask:        nil,
			in:          originalQosVolume,
			out:         originalQosVolume,
			spdk:        []string{`{"id":%d,"error":{"code":0,"message":""},"result":true}`},
			errCode:     codes.OK,
			errMsg:      "",
			existBefore: true,
			missing:     false,
		},
		"malformed name": {
			mask: nil,
			in: &pb.QosVolume{
				Name:          "-ABC-DEF",
				VolumeNameRef: "TBD",
				Limits:        testQosVolume.Limits,
			},
			out:         nil,
			spdk:        []string{},
			errCode:     codes.InvalidArgument,
			errMsg:      fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
			existBefore: false,
			missing:     false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			testEnv := createTestEnvironment(tt.spdk)
			defer testEnv.Close()

			if tt.existBefore {
				testEnv.opiSpdkServer.volumes.qosVolumes[originalQosVolume.Name] = server.ProtoClone(originalQosVolume)
			}

			request := &pb.UpdateQosVolumeRequest{QosVolume: tt.in, UpdateMask: tt.mask, AllowMissing: tt.missing}
			response, err := testEnv.client.UpdateQosVolume(testEnv.ctx, request)

			if !proto.Equal(response, tt.out) {
				t.Error("response: expected", tt.out, "received", response)
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}

			vol := testEnv.opiSpdkServer.volumes.qosVolumes[testQosVolumeName]
			if tt.errCode == codes.OK {
				if !proto.Equal(tt.in, vol) {
					t.Error("expect QoS volume", vol, "is equal to", tt.in)
				}
			} else if tt.existBefore {
				if !proto.Equal(originalQosVolume, vol) {
					t.Error("expect QoS volume", originalQosVolume, "is preserved, received", vol)
				}
			}
		})
	}
}

func TestMiddleEnd_ListQosVolume(t *testing.T) {
	qosVolume0 := &pb.QosVolume{
		Name:          "qos-volume-41",
		VolumeNameRef: "volume-41",
		Limits: &pb.Limits{
			Max: &pb.QosLimit{RwBandwidthMbs: 1},
		},
	}
	qosVolume1 := &pb.QosVolume{
		Name:          "qos-volume-45",
		VolumeNameRef: "volume-45",
		Limits: &pb.Limits{
			Max: &pb.QosLimit{RwBandwidthMbs: 5},
		},
	}
	existingToken := "existing-pagination-token"
	testParent := "todo"
	t.Cleanup(server.CheckTestProtoObjectsNotChanged(qosVolume0, qosVolume1)(t, t.Name()))
	t.Cleanup(checkGlobalTestProtoObjectsNotChanged(t, t.Name()))

	tests := map[string]struct {
		out             []*pb.QosVolume
		existingVolumes map[string]*pb.QosVolume
		errCode         codes.Code
		errMsg          string
		size            int32
		token           string
		in              string
	}{
		"no qos volumes were created": {
			in:              testParent,
			out:             []*pb.QosVolume{},
			existingVolumes: map[string]*pb.QosVolume{},
			errCode:         codes.OK,
			errMsg:          "",
			size:            0,
			token:           "",
		},
		"qos volumes were created": {
			in:  testParent,
			out: []*pb.QosVolume{qosVolume0, qosVolume1},
			existingVolumes: map[string]*pb.QosVolume{
				qosVolume0.Name: qosVolume0,
				qosVolume1.Name: qosVolume1,
			},
			errCode: codes.OK,
			errMsg:  "",
			size:    0,
			token:   "",
		},
		"pagination": {
			in:  testParent,
			out: []*pb.QosVolume{qosVolume0},
			existingVolumes: map[string]*pb.QosVolume{
				qosVolume0.Name: qosVolume0,
				qosVolume1.Name: qosVolume1,
			},
			errCode: codes.OK,
			errMsg:  "",
			size:    1,
			token:   "",
		},
		"pagination offset": {
			in:  testParent,
			out: []*pb.QosVolume{qosVolume1},
			existingVolumes: map[string]*pb.QosVolume{
				qosVolume0.Name: qosVolume0,
				qosVolume1.Name: qosVolume1,
			},
			errCode: codes.OK,
			errMsg:  "",
			size:    1,
			token:   existingToken,
		},
		"pagination negative": {
			in:  testParent,
			out: nil,
			existingVolumes: map[string]*pb.QosVolume{
				qosVolume0.Name: qosVolume0,
				qosVolume1.Name: qosVolume1,
			},
			errCode: codes.InvalidArgument,
			errMsg:  "negative PageSize is not allowed",
			size:    -10,
			token:   "",
		},
		"pagination error": {
			in:  testParent,
			out: nil,
			existingVolumes: map[string]*pb.QosVolume{
				qosVolume0.Name: qosVolume0,
				qosVolume1.Name: qosVolume1,
			},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find pagination token %s", "unknown-pagination-token"),
			size:    0,
			token:   "unknown-pagination-token",
		},
		"no required field": {
			in:              "",
			out:             nil,
			existingVolumes: make(map[string]*pb.QosVolume),
			errCode:         codes.Unknown,
			errMsg:          "missing required field: parent",
			size:            0,
			token:           "",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			testEnv := createTestEnvironment([]string{})
			defer testEnv.Close()
			for k, v := range tt.existingVolumes {
				testEnv.opiSpdkServer.volumes.qosVolumes[k] = server.ProtoClone(v)
			}
			request := &pb.ListQosVolumesRequest{}
			request.Parent = tt.in
			request.PageSize = tt.size
			request.PageToken = tt.token
			testEnv.opiSpdkServer.Pagination[existingToken] = 1

			response, err := testEnv.client.ListQosVolumes(testEnv.ctx, request)

			if !server.EqualProtoSlices(response.GetQosVolumes(), tt.out) {
				t.Error("response: expected", tt.out, "received", response.GetQosVolumes())
			}

			// Empty NextPageToken indicates end of results list
			if tt.size != 1 && response.GetNextPageToken() != "" {
				t.Error("Expected end of results, received non-empty next page token", response.GetNextPageToken())
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}
		})
	}
}

func TestMiddleEnd_GetQosVolume(t *testing.T) {
	t.Cleanup(checkGlobalTestProtoObjectsNotChanged(t, t.Name()))
	tests := map[string]struct {
		in      string
		out     *pb.QosVolume
		errCode codes.Code
		errMsg  string
	}{
		"unknown QoS volume name": {
			in:      server.ResourceIDToVolumeName("unknown-qos-volume-id"),
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %s", server.ResourceIDToVolumeName("unknown-qos-volume-id")),
		},
		"existing QoS volume": {
			in:      testQosVolumeName,
			out:     testQosVolume,
			errCode: codes.OK,
			errMsg:  "",
		},
		"no required field": {
			"",
			nil,
			codes.Unknown,
			"missing required field: name",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			testEnv := createTestEnvironment([]string{})
			defer testEnv.Close()

			testEnv.opiSpdkServer.volumes.qosVolumes[testQosVolumeName] = server.ProtoClone(testQosVolume)

			request := &pb.GetQosVolumeRequest{Name: tt.in}
			response, err := testEnv.client.GetQosVolume(testEnv.ctx, request)

			if !proto.Equal(response, tt.out) {
				t.Error("response: expected", tt.out, "received", response)
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}
		})
	}
}

func TestMiddleEnd_StatsQosVolume(t *testing.T) {
	t.Cleanup(checkGlobalTestProtoObjectsNotChanged(t, t.Name()))
	tests := map[string]struct {
		in      string
		out     *pb.StatsQosVolumeResponse
		spdk    []string
		errCode codes.Code
		errMsg  string
	}{
		"empty QoS volume name is not allowed ": {
			in:      "",
			out:     nil,
			spdk:    []string{},
			errCode: codes.Unknown,
			errMsg:  "missing required field: name",
		},
		"unknown QoS volume Id": {
			in:      "unknown-qos-volume-id",
			out:     nil,
			spdk:    []string{},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %s", "unknown-qos-volume-id"),
		},
		"SPDK call failed": {
			in:      testQosVolumeName,
			out:     nil,
			spdk:    []string{`{"id":%d,"error":{"code":1,"message":"some internal error"}}`},
			errCode: status.Convert(spdk.ErrFailedSpdkCall).Code(),
			errMsg:  status.Convert(spdk.ErrFailedSpdkCall).Message(),
		},
		"SPDK call result false": {
			in:      testQosVolumeName,
			out:     nil,
			spdk:    []string{`{"id":%d,"error":{"code":0,"message":""},"result":{"tick_rate": 3300000000,"ticks": 5,"bdevs":[]}}`},
			errCode: status.Convert(spdk.ErrUnexpectedSpdkCallResult).Code(),
			errMsg:  status.Convert(spdk.ErrUnexpectedSpdkCallResult).Message(),
		},
		"successful QoS volume stats": {
			in: testQosVolumeName,
			out: &pb.StatsQosVolumeResponse{
				Stats: &pb.VolumeStats{
					ReadBytesCount: 36864,
				},
			},
			spdk: []string{
				`{"id":%d,"error":{"code":0,"message":""},"result":{"tick_rate": 3300000000,"ticks": 5,` +
					`"bdevs":[{"name":"` + testQosVolume.VolumeNameRef + `", "bytes_read": 36864}]}}`,
			},
			errCode: codes.OK,
			errMsg:  "",
		},
		"malformed name": {
			in:      "-ABC-DEF",
			out:     nil,
			spdk:    []string{},
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			testEnv := createTestEnvironment(tt.spdk)
			defer testEnv.Close()

			testEnv.opiSpdkServer.volumes.qosVolumes[testQosVolumeName] = server.ProtoClone(testQosVolume)

			request := &pb.StatsQosVolumeRequest{Name: tt.in}
			response, err := testEnv.client.StatsQosVolume(testEnv.ctx, request)

			if !proto.Equal(tt.out, response) {
				t.Error("response: expected", tt.out, "received", response)
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}
		})
	}
}
