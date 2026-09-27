#!/usr/bin/env bash
# M1a N3IWF handler: collapse PreSignalling -> PostSignalling for MN-anchored
# single-round IKE_AUTH (no EAP). Run from ~/L25GC-plus/NFs/n3iwf after the
# config script. Requires MnAnchoredSNEnabled()/GetMnAnchoredKn3iwf() from it.
set -euo pipefail
python3 - <<'PY'
import sys
p="internal/ike/handler.go"
s=open(p).read()
old=('\t\tikeSecurityAssociation.TrafficSelectorResponder = trafficSelectorResponder\n'
     '\n'
     '\t\tresponseIKEPayload.Reset()\n')
new=('\t\tikeSecurityAssociation.TrafficSelectorResponder = trafficSelectorResponder\n'
     '\n'
     '\t\tif cfg.MnAnchoredSNEnabled() && authentication != nil {\n'
     '\t\t\t// MN-anchored single-round IKE_AUTH (no EAP): the UE\'s message already\n'
     '\t\t\t// carries IDi(SUPI)+SA+TSi+TSr+AUTH+CFG, and the SUPI IDi is already\n'
     '\t\t\t// folded into InitiatorSignedOctets above. Allocate the UE context,\n'
     '\t\t\t// key it from the delivered K_N3IWF_SN, advance to PostSignalling and\n'
     '\t\t\t// re-dispatch this same message so PostSignalling verifies AUTH and\n'
     '\t\t\t// completes the CP child SA. No NGAP/EAP relay is used.\n'
     '\t\t\tkn3iwf := cfg.GetMnAnchoredKn3iwf()\n'
     '\t\t\tif len(kn3iwf) != 32 {\n'
     '\t\t\t\tikeLog.Errorf("MN-anchored SN: no/invalid K_N3IWF_SN in config")\n'
     '\t\t\t\treturn\n'
     '\t\t\t}\n'
     '\t\t\tikeUe := n3iwfCtx.NewN3iwfIkeUe(ikeSecurityAssociation.LocalSPI)\n'
     '\t\t\tikeSecurityAssociation.IkeUE = ikeUe\n'
     '\t\t\tikeUe.N3IWFIKESecurityAssociation = ikeSecurityAssociation\n'
     '\t\t\tikeUe.IKEConnection = ikeSecurityAssociation.IKEConnection\n'
     '\t\t\tikeUe.Kn3iwf = kn3iwf\n'
     '\t\t\tikeSecurityAssociation.State = PostSignalling\n'
     '\t\t\ts.HandleIKEAUTH(udpConn, n3iwfAddr, ueAddr, message, ikeSecurityAssociation)\n'
     '\t\t\treturn\n'
     '\t\t}\n'
     '\n'
     '\t\tresponseIKEPayload.Reset()\n')
n=s.count(old)
if n!=1: sys.exit(f"[handler.go] anchor matched {n} (want 1) - aborting")
open(p,"w").write(s.replace(old,new)); print("[handler.go] OK")
PY
gofmt -w internal/ike/handler.go
echo "handler done"
