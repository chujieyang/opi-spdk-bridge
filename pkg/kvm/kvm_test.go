// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Intel Corporation

// Package kvm automates plugging of SPDK devices to a QEMU instance
package kvm

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pc "github.com/opiproject/opi-api/common/v1/gen/go"
	pb "github.com/opiproject/opi-api/storage/v1alpha1/gen/go"
	"github.com/opiproject/opi-spdk-bridge/pkg/frontend"
	"github.com/opiproject/opi-spdk-bridge/pkg/models"
	"github.com/opiproject/opi-spdk-bridge/pkg/server"
	"github.com/ulule/deepcopier"
	"google.golang.org/protobuf/proto"
)

var (
	alwaysSuccessfulJSONRPC    = stubJSONRRPC{nil}
	alwaysFailingJSONRPC       = stubJSONRRPC{errors.New("stub error")}
	testVirtioBlkID            = "virtio-blk-42"
	testCreateVirtioBlkRequest = &pb.CreateVirtioBlkRequest{VirtioBlk: &pb.VirtioBlk{
		Id:       &pc.ObjectKey{Value: testVirtioBlkID},
		PcieId:   &pb.PciEndpoint{PhysicalFunction: 42},
		VolumeId: &pc.ObjectKey{Value: "Malloc42"},
		MaxIoQps: 1,
	}}
	testDeleteVirtioBlkRequest = &pb.DeleteVirtioBlkRequest{Name: testVirtioBlkID}
	genericQmpError            = `{"error": {"class": "GenericError", "desc": "some error"}}` + "\n"
	genericQmpOk               = `{"return": {}}` + "\n"

	qmpServerOperationTimeout = 500 * time.Millisecond
	qmplibTimeout             = 250 * time.Millisecond
)

type stubJSONRRPC struct {
	err error
}

func (s stubJSONRRPC) Call(method string, args, result interface{}) error {
	if method == "vhost_create_blk_controller" {
		if s.err == nil {
			resultCreateVirtioBLk, ok := result.(*models.VhostCreateBlkControllerResult)
			if !ok {
				log.Panicf("Unexpected type for virtio-blk device creation result")
			}
			*resultCreateVirtioBLk = models.VhostCreateBlkControllerResult(true)
		}
		return s.err
	} else if method == "vhost_delete_controller" {
		if s.err == nil {
			resultDeleteVirtioBLk, ok := result.(*models.VhostDeleteControllerResult)
			if !ok {
				log.Panicf("Unexpected type for virtio-blk device deletion result")
			}
			*resultDeleteVirtioBLk = models.VhostDeleteControllerResult(true)
		}
		return s.err
	} else {
		return s.err
	}
}

type mockCall struct {
	response     string
	event        string
	expectedArgs []string
}

type mockQmpServer struct {
	socket     net.Listener
	testDir    string
	socketPath string

	greeting                string
	capabilitiesNegotiation mockCall
	expectedCalls           []mockCall
	callIndex               uint32

	test *testing.T
	mu   sync.Mutex
}

func startMockQmpServer(t *testing.T) *mockQmpServer {
	s := &mockQmpServer{}
	s.greeting =
		`{"QMP":{"version":{"qemu":{"micro":50,"minor":0,"major":7},"package":""},"capabilities":[]}}`
	s.capabilitiesNegotiation = mockCall{
		response: genericQmpOk,
		expectedArgs: []string{
			`"execute":"qmp_capabilities"`,
		},
	}

	testDir, err := os.MkdirTemp("", "opi-spdk-kvm-test")
	if err != nil {
		log.Panic(err.Error())
	}
	s.testDir = testDir

	s.socketPath = filepath.Join(s.testDir, "qmp.sock")
	socket, err := net.Listen("unix", s.socketPath)
	if err != nil {
		log.Panic(err.Error())
	}
	s.socket = socket
	s.test = t

	go func() {
		conn, err := s.socket.Accept()
		if err != nil {
			return
		}
		err = conn.SetDeadline(time.Now().Add(qmpServerOperationTimeout))
		if err != nil {
			log.Panicf("Failed to set deadline: %v", err)
		}

		s.write(s.greeting, conn)
		s.handleCall(s.capabilitiesNegotiation, conn)
		for _, call := range s.expectedCalls {
			s.handleExpectedCall(call, conn)
		}
	}()

	return s
}

func (s *mockQmpServer) Stop() {
	if s.socket != nil {
		if err := s.socket.Close(); err != nil {
			log.Panicf("Failed to close socket: %v", err)
		}
	}
	if err := os.RemoveAll(s.testDir); err != nil {
		log.Panicf("Failed to delete test dir: %v", err)
	}
}

func (s *mockQmpServer) ExpectAddChardev(id string) *mockQmpServer {
	s.expectedCalls = append(s.expectedCalls, mockCall{
		response: `{"return": {"pty": "/tmp/dev/pty/42"}}` + "\n",
		expectedArgs: []string{
			`"execute":"chardev-add"`,
			`"id":"` + id + `"`,
			`"path":"` + filepath.Join(s.testDir, id) + `"`,
		},
	})
	return s
}

func (s *mockQmpServer) ExpectAddVirtioBlk(id string, chardevID string) *mockQmpServer {
	s.expectedCalls = append(s.expectedCalls, mockCall{
		response: genericQmpOk,
		expectedArgs: []string{
			`"execute":"device_add"`,
			`"driver":"vhost-user-blk-pci"`,
			`"id":"` + id + `"`,
			`"chardev":"` + chardevID + `"`,
		},
	})
	return s
}

func (s *mockQmpServer) ExpectDeleteChardev(id string) *mockQmpServer {
	s.expectedCalls = append(s.expectedCalls, mockCall{
		response: genericQmpOk,
		expectedArgs: []string{
			`"execute":"chardev-remove"`,
			`"id":"` + id + `"`,
		},
	})
	return s
}

func (s *mockQmpServer) ExpectDeleteVirtioBlkWithEvent(id string) *mockQmpServer {
	s.ExpectDeleteVirtioBlk(id)
	s.expectedCalls[len(s.expectedCalls)-1].event =
		`{"event":"DEVICE_DELETED","data":{"path":"/some/path","device":"` +
			id + `"},"timestamp":{"seconds":1,"microseconds":2}}` + "\n"
	return s
}

func (s *mockQmpServer) ExpectDeleteVirtioBlk(id string) *mockQmpServer {
	s.expectedCalls = append(s.expectedCalls, mockCall{
		response: genericQmpOk,
		expectedArgs: []string{
			`"execute":"device_del"`,
			`"id":"` + id + `"`,
		},
	})
	return s
}

func (s *mockQmpServer) WithErrorResponse() *mockQmpServer {
	if len(s.expectedCalls) == 0 {
		log.Panicf("No instance to add a QMP error")
	}
	s.expectedCalls[len(s.expectedCalls)-1].response = genericQmpError
	return s
}

func (s *mockQmpServer) WereExpectedCallsPerformed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	numberOfPerformedCalls := s.callIndex
	numberOfExpectedCalls := len(s.expectedCalls)
	ok := int(numberOfPerformedCalls) == numberOfExpectedCalls
	if !ok {
		log.Printf("Not all expected calls are performed. Expected calls %v: %v. Index: %v",
			numberOfPerformedCalls, s.expectedCalls, numberOfPerformedCalls)
	}
	return ok
}

func (s *mockQmpServer) handleExpectedCall(call mockCall, conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handleCall(call, conn)
	s.callIndex++
}

func (s *mockQmpServer) handleCall(call mockCall, conn net.Conn) {
	req := s.read(conn)
	for _, expectedArg := range call.expectedArgs {
		if !strings.Contains(req, expectedArg) {
			s.test.Errorf("Expected to find argument %v in %v", expectedArg, req)
		}
	}
	s.write(call.response, conn)
	if call.event != "" {
		time.Sleep(time.Millisecond * 1)
		s.write(call.event, conn)
	}
}

func (s *mockQmpServer) write(data string, conn net.Conn) {
	log.Println("QMP server send:", data)
	_, err := conn.Write([]byte(data))
	if err != nil {
		log.Panicf("QMP server failed to write: %v", data)
	}
}

func (s *mockQmpServer) read(conn net.Conn) string {
	buf := make([]byte, 512)
	_, err := conn.Read(buf)
	if err != nil {
		log.Panicf("QMP server failed to read")
	}
	data := string(buf)
	log.Println("QMP server got :", data)
	return data
}

func TestCreateVirtioBlk(t *testing.T) {
	expectNotNilOut := &pb.VirtioBlk{}
	if deepcopier.Copy(testCreateVirtioBlkRequest.VirtioBlk).To(expectNotNilOut) != nil {
		log.Panicf("Failed to copy structure")
	}

	tests := map[string]struct {
		expectAddChardev      bool
		expectAddChardevError bool

		expectAddVirtioBlk      bool
		expectAddVirtioBlkError bool

		expectDeleteChardev bool

		jsonRPC              server.JSONRPC
		expectError          error
		nonDefaultQmpAddress string

		out *pb.VirtioBlk
	}{
		"valid virtio-blk creation": {
			expectAddChardev:   true,
			expectAddVirtioBlk: true,
			jsonRPC:            alwaysSuccessfulJSONRPC,
			out:                expectNotNilOut,
		},
		"spdk failed to create virtio-blk": {
			jsonRPC:     alwaysFailingJSONRPC,
			expectError: server.ErrFailedSpdkCall,
		},
		"qemu chardev add failed": {
			expectAddChardevError: true,
			jsonRPC:               alwaysSuccessfulJSONRPC,
			expectError:           errAddChardevFailed,
		},
		"qemu device add failed": {
			expectAddChardev:        true,
			expectAddVirtioBlkError: true,
			expectDeleteChardev:     true,
			jsonRPC:                 alwaysSuccessfulJSONRPC,
			expectError:             errAddDeviceFailed,
		},
		"failed to create monitor": {
			nonDefaultQmpAddress: "/dev/null",
			jsonRPC:              alwaysSuccessfulJSONRPC,
			expectError:          errMonitorCreation,
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			opiSpdkServer := frontend.NewServer(test.jsonRPC)
			qmpServer := startMockQmpServer(t)
			defer qmpServer.Stop()
			qmpAddress := qmpServer.socketPath
			if test.nonDefaultQmpAddress != "" {
				qmpAddress = test.nonDefaultQmpAddress
			}
			kvmServer := NewServer(opiSpdkServer, qmpAddress, qmpServer.testDir)
			kvmServer.timeout = qmplibTimeout

			if test.expectAddChardev {
				qmpServer.ExpectAddChardev(testVirtioBlkID)
			}
			if test.expectAddChardevError {
				qmpServer.ExpectAddChardev(testVirtioBlkID).WithErrorResponse()
			}
			if test.expectAddVirtioBlk {
				qmpServer.ExpectAddVirtioBlk(testVirtioBlkID, testVirtioBlkID)
			}
			if test.expectAddVirtioBlkError {
				qmpServer.ExpectAddVirtioBlk(testVirtioBlkID, testVirtioBlkID).WithErrorResponse()
			}
			if test.expectDeleteChardev {
				qmpServer.ExpectDeleteChardev(testVirtioBlkID)
			}

			out, err := kvmServer.CreateVirtioBlk(context.Background(), testCreateVirtioBlkRequest)
			if !errors.Is(err, test.expectError) {
				t.Errorf("Expected error %v, got %v", test.expectError, err)
			}
			gotOut, _ := proto.Marshal(out)
			wantOut, _ := proto.Marshal(test.out)
			if !bytes.Equal(gotOut, wantOut) {
				t.Errorf("Expected out %v, got %v", &test.out, out)
			}
			if !qmpServer.WereExpectedCallsPerformed() {
				t.Errorf("Not all expected calls were performed")
			}
		})
	}
}

func TestDeleteVirtioBlk(t *testing.T) {
	tests := map[string]struct {
		expectDeleteVirtioBlk          bool
		expectDeleteVirtioBlkWithEvent bool
		expectDeleteVirtioBlkError     bool

		expectDeleteChardev      bool
		expectDeleteChardevError bool

		jsonRPC              server.JSONRPC
		expectError          error
		nonDefaultQmpAddress string
	}{
		"valid virtio-blk deletion": {
			expectDeleteVirtioBlkWithEvent: true,
			expectDeleteChardev:            true,
			jsonRPC:                        alwaysSuccessfulJSONRPC,
		},
		"qemu device delete failed": {
			expectDeleteVirtioBlkError: true,
			expectDeleteChardev:        true,
			jsonRPC:                    alwaysSuccessfulJSONRPC,
			expectError:                errDevicePartiallyDeleted,
		},
		"qemu device delete failed by timeout": {
			expectDeleteVirtioBlk: true,
			expectDeleteChardev:   true,
			jsonRPC:               alwaysSuccessfulJSONRPC,
			expectError:           errDevicePartiallyDeleted,
		},
		"qemu chardev delete failed": {
			expectDeleteVirtioBlkWithEvent: true,
			expectDeleteChardevError:       true,
			jsonRPC:                        alwaysSuccessfulJSONRPC,
			expectError:                    errDevicePartiallyDeleted,
		},
		"spdk failed to delete virtio-blk": {
			expectDeleteVirtioBlkWithEvent: true,
			expectDeleteChardev:            true,
			jsonRPC:                        alwaysFailingJSONRPC,
			expectError:                    errDevicePartiallyDeleted,
		},
		"all qemu and spdk calls failed": {
			expectDeleteVirtioBlkError: true,
			expectDeleteChardevError:   true,
			jsonRPC:                    alwaysFailingJSONRPC,
			expectError:                errDeviceNotDeleted,
		},
		"failed to create monitor": {
			nonDefaultQmpAddress: "/dev/null",
			jsonRPC:              alwaysSuccessfulJSONRPC,
			expectError:          errMonitorCreation,
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			opiSpdkServer := frontend.NewServer(test.jsonRPC)
			opiSpdkServer.Virt.BlkCtrls[testVirtioBlkID] = testCreateVirtioBlkRequest.VirtioBlk
			qmpServer := startMockQmpServer(t)
			defer qmpServer.Stop()
			qmpAddress := qmpServer.socketPath
			if test.nonDefaultQmpAddress != "" {
				qmpAddress = test.nonDefaultQmpAddress
			}
			kvmServer := NewServer(opiSpdkServer, qmpAddress, qmpServer.testDir)
			kvmServer.timeout = qmplibTimeout

			if test.expectDeleteVirtioBlkWithEvent {
				qmpServer.ExpectDeleteVirtioBlkWithEvent(testVirtioBlkID)
			}
			if test.expectDeleteVirtioBlk {
				qmpServer.ExpectDeleteVirtioBlk(testVirtioBlkID)
			}
			if test.expectDeleteVirtioBlkError {
				qmpServer.ExpectDeleteVirtioBlk(testVirtioBlkID).WithErrorResponse()
			}
			if test.expectDeleteChardev {
				qmpServer.ExpectDeleteChardev(testVirtioBlkID)
			}
			if test.expectDeleteChardevError {
				qmpServer.ExpectDeleteChardev(testVirtioBlkID).WithErrorResponse()
			}

			_, err := kvmServer.DeleteVirtioBlk(context.Background(), testDeleteVirtioBlkRequest)
			if !errors.Is(err, test.expectError) {
				t.Errorf("Expected %v, got %v", test.expectError, err)
			}
			if !qmpServer.WereExpectedCallsPerformed() {
				t.Errorf("Not all expected calls were performed")
			}
		})
	}
}
