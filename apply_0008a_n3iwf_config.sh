#!/usr/bin/env bash
# M1a N3IWF config: add MnAnchoredSN {enable, kn3iwfHex} + getters.
# Run from ~/L25GC-plus/NFs/n3iwf (branch feat/mn-anchored-sn).
set -euo pipefail
python3 - <<'PY'
import sys
p="pkg/factory/config.go"
s=open(p).read()
def repl(old,new,label):
    global s
    n=s.count(old)
    if n!=1: sys.exit(f"[config.go] anchor '{label}' matched {n} (want 1) - aborting")
    s=s.replace(old,new)
repl('\t"errors"\n\t"fmt"\n\t"net"\n',
     '\t"encoding/hex"\n\t"errors"\n\t"fmt"\n\t"net"\n',"hex import")
repl('\tChildSARekey         *ChildSARekeyConfig `yaml:"childSARekey,omitempty" valid:"optional"`\n}\n',
     '\tChildSARekey         *ChildSARekeyConfig `yaml:"childSARekey,omitempty" valid:"optional"`\n'
     '\tMnAnchoredSN         *MnAnchoredSN       `yaml:"mnAnchoredSN,omitempty" valid:"optional"`\n}\n',"field")
anchor='func appendInvalid(err error) error {'
repl(anchor,
     'type MnAnchoredSN struct {\n'
     '\tEnable    bool   `yaml:"enable" valid:"optional"`\n'
     '\tKn3iwfHex string `yaml:"kn3iwfHex,omitempty" valid:"optional,hexadecimal"`\n'
     '}\n\n'
     'func (c *Config) MnAnchoredSNEnabled() bool {\n'
     '\tc.RLock()\n\tdefer c.RUnlock()\n'
     '\treturn c.Configuration != nil && c.Configuration.MnAnchoredSN != nil && c.Configuration.MnAnchoredSN.Enable\n}\n\n'
     'func (c *Config) GetMnAnchoredKn3iwf() []byte {\n'
     '\tc.RLock()\n\tdefer c.RUnlock()\n'
     '\tif c.Configuration == nil || c.Configuration.MnAnchoredSN == nil {\n\t\treturn nil\n\t}\n'
     '\tk, err := hex.DecodeString(c.Configuration.MnAnchoredSN.Kn3iwfHex)\n'
     '\tif err != nil {\n\t\treturn nil\n\t}\n\treturn k\n}\n\n'
     + anchor,"type+getters")
open(p,"w").write(s); print("[config.go] OK")
PY
gofmt -w pkg/factory/config.go
echo "config done — build after the handler edit lands"
