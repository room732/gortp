package rtp

import (
	"fmt"
	"log"
	"net"
	"time"
)

// TransportTCP implements the interfaces TransportRecv and TransportWrite for RTP transports.
type TransportTCP struct {
	TransportCommon
	callUpper                     TransportRecv
	toLower                       TransportWrite
	dataConn, ctrlConn            net.Conn
	localAddrRtp, localAddrRtcp   *net.TCPAddr
	remoteAddrRtp, remoteAddrRtcp *net.TCPAddr
}

// NewTransportTCP creates a new RTP transport for TCP.
//
// addr - The TCP socket's local IP address
//
// port - The port number of the RTP data port. This must be an even port number.
//        The following odd port number is the control (RTCP) port.
//
func NewTransportTCP(addr *net.IPAddr, port int) (*TransportTCP, error) {
	tp := new(TransportTCP)
	tp.callUpper = tp
	tp.localAddrRtp = &net.TCPAddr{IP: addr.IP, Port: port}
	tp.localAddrRtcp = &net.TCPAddr{IP: addr.IP, Port: port + 1}
	return tp, nil
}

// ListenOnTransports listens for incoming RTP and RTCP packets addressed
// to this transport.
//
func (tp *TransportTCP) ListenOnTransports() (err error) {
	go func() {
		log.Println("Start listening...")
		ln, err := net.ListenTCP(tp.localAddrRtp.Network(), tp.localAddrRtp)
		if err != nil {
			return
		}
		log.Printf("Listen on: %s", ln.Addr())
		tp.dataConn, err = ln.AcceptTCP()
		log.Printf("Accept connection from: %s", tp.dataConn.RemoteAddr())
		tp.remoteAddrRtp, _ = net.ResolveTCPAddr(tp.dataConn.RemoteAddr().Network(), tp.dataConn.RemoteAddr().String())
		ln.Close()
		go tp.readDataPacket()
	}()
	return
}

func (tp *TransportTCP) SetCallUpper(upper TransportRecv) {
	tp.callUpper = upper
}

func (tp *TransportTCP) OnRecvData(rp *DataPacket) bool {
	fmt.Printf("TransportTCP: no registered upper layer RTP packet handler\n")
	return false
}

func (tp *TransportTCP) OnRecvCtrl(rp *CtrlPacket) bool {
	fmt.Printf("TransportTCP: no registered upper layer RTCP packet handler\n")
	return false
}

func (tp *TransportTCP) CloseRecv() {
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

// SetEndChannel receives and set the channel to signal back after network socket was closed and receive loop terminated.
func (tp *TransportTCP) SetEndChannel(ch TransportEnd) {
	tp.transportEnd = ch
}

func (tp *TransportTCP) readDataPacket() {
	var buf [defaultBufferSize]byte

	tp.dataRecvStop = false
	for {
		tp.dataConn.SetReadDeadline(time.Now().Add(20 * time.Millisecond)) // 20 ms, re-test and remove after Go issue 2116 is solved
		n, err := tp.dataConn.Read(buf[0:])
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
		rp.fromAddr.IpAddr = tp.remoteAddrRtp.IP
		rp.fromAddr.DataPort = tp.remoteAddrRtp.Port
		rp.fromAddr.CtrlPort = 0
		rp.inUse = n-2
		copy(rp.buffer, buf[2:n])
		if tp.callUpper != nil {
			tp.callUpper.OnRecvData(rp)
		}
	}
	tp.dataConn.Close()
	tp.transportEnd <- DataTransportRecvStopped
}
