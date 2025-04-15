// Copyright (C) 2011 Werner Dittmann
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//
// Authors: Werner Dittmann <Werner.Dittmann@t-online.de>
//

package rtp

import (
	"fmt"
	"net"
	"syscall"
	"time"

	"github.com/room732/gortp/iana"

	"golang.org/x/net/ipv4"
)

// TransportMulticast implements the interfaces RtpTransportRecv and RtpTransportWrite for RTP transports.
type TransportMulticast struct {
	TransportCommon
	callUpper                   TransportRecv
	toLower                     TransportWrite
	dataConn, ctrlConn          *net.UDPConn
	localAddrRtp, localAddrRtcp *net.UDPAddr
}

// NewTransportMulticast creates a new RTP transport for UPD.
//
// addr - The UPD socket's local IP address
//
// port - The port number of the RTP data port. This must be an even port number.
//
//	The following odd port number is the control (RTCP) port.
func NewTransportMulticast(addr *net.IPAddr, port int) (*TransportMulticast, error) {
	tp := new(TransportMulticast)
	tp.callUpper = tp
	tp.localAddrRtp = &net.UDPAddr{addr.IP, port, ""}
	tp.localAddrRtcp = &net.UDPAddr{addr.IP, port + 1, ""}
	return tp, nil
}

// ListenOnTransports listens for incoming RTP and RTCP packets addressed
// to this transport.
func (tp *TransportMulticast) ListenOnTransports() (err error) {
	tp.dataConn, err = net.ListenMulticastUDP("udp", nil, tp.localAddrRtp)
	// tp.dataConn, err = net.ListenUDP(tp.localAddrRtp.Network(), tp.localAddrRtp)
	if err != nil {
		fmt.Println("ListenMulticastUDP for RTP err:", err)
		return
	}
	if err := setReuseAddr(tp.dataConn); err != nil {
		fmt.Println("setReusePort for RTP err:", err)
	}

	p := ipv4.NewConn(tp.dataConn)
	if err = p.SetTOS(iana.DiffServAF41); err != nil {
		fmt.Printf("TransportMulticast: failed to set TOS marking on dataConn\n")
	}
	go tp.readDataPacket()

	// TODO rm ctrl conn for mcast ?
	// tp.ctrlConn, err = net.ListenMulticastUDP("udp", nil, tp.localAddrRtcp)
	// if err != nil {
	// 	fmt.Println("ListenMulticastUDP for RTCP err:", err)
	// 	tp.dataConn.Close()
	// 	tp.dataConn = nil
	// 	return
	// }
	// if err := setReuseAddr(tp.ctrlConn); err != nil {
	// 	fmt.Println("setReusePort for RTCP err:", err)
	// }
	// go tp.readCtrlPacket()
	return nil
}

func setReuseAddr(conn *net.UDPConn) error {
	rc, err := conn.SyscallConn()
	if err != nil {
		return err
	}

	err = rc.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})

	return err
}

// *** The following methods implement the rtp.TransportRecv interface.

// SetCallUpper implements the rtp.TransportRecv SetCallUpper method.
func (tp *TransportMulticast) SetCallUpper(upper TransportRecv) {
	tp.callUpper = upper
}

// OnRecvRtp implements the rtp.TransportRecv OnRecvRtp method.
//
// TransportMulticast does not implement any processing because it is the lowest
// layer and expects an upper layer to receive data.
func (tp *TransportMulticast) OnRecvData(rp *DataPacket) bool {
	fmt.Printf("TransportMulticast: no registered upper layer RTP packet handler\n")
	return false
}

// OnRecvRtcp implements the rtp.TransportRecv OnRecvRtcp method.
//
// TransportMulticast does not implement any processing because it is the lowest
// layer and expects an upper layer to receive data.
func (tp *TransportMulticast) OnRecvCtrl(rp *CtrlPacket) bool {
	fmt.Printf("TransportMulticast: no registered upper layer RTCP packet handler\n")
	return false
}

// CloseRecv implements the rtp.TransportRecv CloseRecv method.
func (tp *TransportMulticast) CloseRecv() {
	//
	// The correct way to do it is to close the UDP connection after setting the
	// stop flags to true. However, until issue 2116 is solved just set the flags
	// and rely on the read timeout in the read packet functions
	//
	tp.dataRecvStop = true
	tp.ctrlRecvStop = true

	//    err := tp.rtpConn.Close()
	//    if err != nil {
	//        fmt.Printf("Close failed: %s\n", err.String())
	//    }
	//    tp.rtcpConn.Close()
}

// setEndChannel receives and set the channel to signal back after network socket was closed and receive loop terminated.
func (tp *TransportMulticast) SetEndChannel(ch TransportEnd) {
	tp.transportEnd = ch
}

// *** The following methods implement the rtp.TransportWrite interface.

// SetToLower implements the rtp.TransportWrite SetToLower method.
//
// Usually TransportMulticast is already the lowest layer.
func (tp *TransportMulticast) SetToLower(lower TransportWrite) {
	tp.toLower = lower
}

// WriteRtpTo implements the rtp.TransportWrite WriteRtpTo method.
func (tp *TransportMulticast) WriteDataTo(rp *DataPacket, addr *Address) (n int, err error) {
	return tp.dataConn.WriteToUDP(rp.buffer[0:rp.inUse], &net.UDPAddr{addr.IpAddr, addr.DataPort, ""})
}

// WriteRtcpTo implements the rtp.TransportWrite WriteRtcpTo method.
func (tp *TransportMulticast) WriteCtrlTo(rp *CtrlPacket, addr *Address) (n int, err error) {
	//return tp.ctrlConn.WriteToUDP(rp.buffer[0:rp.inUse], &net.UDPAddr{addr.IpAddr, addr.CtrlPort, ""})
	// TODO: big hack - send back RTCP packets (SR) in RTP data port, since hole punching is only
	// done on the RTP data port...
	return tp.dataConn.WriteToUDP(rp.buffer[0:rp.inUse], &net.UDPAddr{addr.IpAddr, addr.DataPort, ""})
}

// CloseWrite implements the rtp.TransportWrite CloseWrite method.
//
// Nothing to do for TransportMulticast. The application shall close the receiver (CloseRecv()), this will
// close the local UDP socket.
func (tp *TransportMulticast) CloseWrite() {
}

// *** Local functions and methods.

// Here the local RTP and RTCP UDP network receivers. The ListenOnTransports() starts them
// as go functions. The functions just receive data from the network, copy it into
// the packet buffers and forward the packets to the next upper layer via callback
// if callback is not nil

func (tp *TransportMulticast) readDataPacket() {
	var buf [defaultBufferSize]byte

	tp.dataRecvStop = false
	for {
		tp.dataConn.SetReadDeadline(time.Now().Add(20 * time.Millisecond)) // 20 ms, re-test and remove after Go issue 2116 is solved
		n, addr, err := tp.dataConn.ReadFromUDP(buf[0:])
		if tp.dataRecvStop {
			break
		}
		if e, ok := err.(net.Error); ok && e.Timeout() {
			continue
		}
		if err != nil {
			break
		}
		rp := newDataPacket()
		rp.fromAddr.IpAddr = addr.IP
		rp.fromAddr.DataPort = addr.Port
		rp.fromAddr.CtrlPort = 0
		rp.inUse = n
		copy(rp.buffer, buf[0:n])

		if tp.callUpper != nil {
			tp.callUpper.OnRecvData(rp)
		}
	}
	tp.dataConn.Close()
	tp.transportEnd <- DataTransportRecvStopped
}

func (tp *TransportMulticast) readCtrlPacket() {
	var buf [defaultBufferSize]byte

	tp.ctrlRecvStop = false
	for {
		tp.ctrlConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)) // 100 ms, re-test and remove after Go issue 2116 is solved
		n, addr, err := tp.ctrlConn.ReadFromUDP(buf[0:])
		if tp.ctrlRecvStop {
			break
		}
		if e, ok := err.(net.Error); ok && e.Timeout() {
			continue
		}
		if err != nil {
			break
		}
		rp, _ := newCtrlPacket()
		rp.fromAddr.IpAddr = addr.IP
		rp.fromAddr.CtrlPort = addr.Port
		rp.fromAddr.DataPort = 0
		rp.inUse = n
		copy(rp.buffer, buf[0:n])

		if tp.callUpper != nil {
			tp.callUpper.OnRecvCtrl(rp)
		}
	}
	tp.ctrlConn.Close()
	tp.transportEnd <- CtrlTransportRecvStopped
}
