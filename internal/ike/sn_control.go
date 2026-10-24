package ike

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"

	n3iwf_context "github.com/free5gc/n3iwf/internal/context"
	"github.com/free5gc/n3iwf/internal/logger"
)

// SN-Add control: the core (AMF), co-located on this node, delivers the
// registration-derived K_N3IWF_SN plus the SN-leg PDU session params over a
// local unix socket. Keyed by SUPI (the IKE IDi the UE presents).

type snAddRequest struct {
	SUPI              string `json:"supi"`
	Kn3iwfHex         string `json:"kn3iwfHex"`
	PDUSessionID      uint32 `json:"pduSessionID"`
	QFIs              []int  `json:"qfis"`
	UplinkTEID        uint32 `json:"uplinkTEID"`
	UPFN3Address      string `json:"upfN3Address"`
	SecurityIntegrity bool   `json:"securityIntegrity"`
	SecurityCipher    bool   `json:"securityCipher"`
}

type snAddReply struct {
	Status       string `json:"status"`
	DownlinkTEID uint32 `json:"downlinkTEID"`
	N3IWFN3Addr  string `json:"n3iwfN3Addr"`
	UEInnerIP    string `json:"ueInnerIP"`
	Error        string `json:"error,omitempty"`
}

type snPendingEntry struct {
	kn3iwf []byte
	params MnAnchoredSessionParams
	done   chan snAddReply
}

var (
	snPendingMu sync.Mutex
	snPending   = map[string]*snPendingEntry{}
)

func snStore(supi string, e *snPendingEntry) {
	snPendingMu.Lock()
	snPending[supi] = e
	snPendingMu.Unlock()
}

func snLookup(supi string) *snPendingEntry {
	snPendingMu.Lock()
	defer snPendingMu.Unlock()
	return snPending[supi]
}

func snDelete(supi string) {
	snPendingMu.Lock()
	delete(snPending, supi)
	snPendingMu.Unlock()
}

// snComplete releases a blocked SN-Add caller once the SN leg is up.
func snComplete(supi string, dlTeid uint32, n3addr, ueIP string) {
	e := snLookup(supi)
	if e == nil {
		return
	}
	select {
	case e.done <- snAddReply{Status: "ok", DownlinkTEID: dlTeid, N3IWFN3Addr: n3addr, UEInnerIP: ueIP}:
	default:
	}
}

// mnParamsFor returns the SN-leg params for this UE: the SN-Add entry if present,
// else the config testSession shim (isolated test / M1b compatibility).
func (s *Server) mnParamsFor(ikeUe *n3iwf_context.N3IWFIkeUe) *MnAnchoredSessionParams {
	if e := snLookup(ikeUe.MnSupi); e != nil {
		p := e.params
		return &p
	}
	if ts := s.Config().GetMnAnchoredTestSession(); ts != nil {
		qfis := make([]uint8, 0, len(ts.QFIs))
		for _, q := range ts.QFIs {
			qfis = append(qfis, uint8(q))
		}
		return &MnAnchoredSessionParams{
			PDUSessionID:      ts.PDUSessionID,
			QFIs:              qfis,
			UplinkTEID:        ts.UplinkTEID,
			UPFN3Address:      ts.UPFN3Address,
			SecurityIntegrity: ts.SecurityIntegrity,
			SecurityCipher:    ts.SecurityCipher,
		}
	}
	return nil
}

func (s *Server) serveSNControl() {
	ikeLog := logger.IKELog
	path := s.Config().GetSNControlSocket()
	if path == "" {
		return
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		ikeLog.Errorf("SN-control: listen %s: %v", path, err)
		return
	}
	ikeLog.Infof("SN-control listening on %s", path)
	for {
		conn, err := ln.Accept()
		if err != nil {
			ikeLog.Errorf("SN-control accept: %v", err)
			return
		}
		go s.handleSNConn(conn)
	}
}

func (s *Server) handleSNConn(conn net.Conn) {
	ikeLog := logger.IKELog
	defer conn.Close()
	var req snAddRequest
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(snAddReply{Status: "error", Error: "decode: " + err.Error()})
		return
	}
	kn3iwf, err := hex.DecodeString(req.Kn3iwfHex)
	if err != nil || len(kn3iwf) != 32 {
		_ = json.NewEncoder(conn).Encode(snAddReply{Status: "error", Error: "bad kn3iwfHex (need 32 bytes)"})
		return
	}
	qfis := make([]uint8, 0, len(req.QFIs))
	for _, q := range req.QFIs {
		qfis = append(qfis, uint8(q))
	}
	e := &snPendingEntry{
		kn3iwf: kn3iwf,
		params: MnAnchoredSessionParams{
			PDUSessionID:      req.PDUSessionID,
			QFIs:              qfis,
			UplinkTEID:        req.UplinkTEID,
			UPFN3Address:      req.UPFN3Address,
			SecurityIntegrity: req.SecurityIntegrity,
			SecurityCipher:    req.SecurityCipher,
		},
		done: make(chan snAddReply, 1),
	}
	snStore(req.SUPI, e)
	ikeLog.Infof("SN-control: pending SN-Add SUPI=%s PDU=%d QFIs=%v UL-TEID=%d UPF=%s",
		req.SUPI, req.PDUSessionID, qfis, req.UplinkTEID, req.UPFN3Address)

	select {
	case reply := <-e.done:
		_ = json.NewEncoder(conn).Encode(reply)
	case <-time.After(30 * time.Second):
		snDelete(req.SUPI)
		_ = json.NewEncoder(conn).Encode(snAddReply{Status: "error", Error: "timeout waiting for SN leg"})
	}
}
