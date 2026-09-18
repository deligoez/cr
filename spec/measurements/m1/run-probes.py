#!/usr/bin/env python3
"""Pass 2: run every probe the roles proposed, then attach probe ids to the
records they name.

Reads probes-proposed/*.ndjson (one manifest line per proposed probe), runs
`cr probe run` for each in the probe-pass state root, writes probes-run.ndjson
(one line per attempt: the manifest, cr's answer or its refusal), and rewrites
the role output files under the fanout so a record named by a probe carries
`"probe": "p<n>"` (the first probe run for it whose result is not error/timeout;
cr grades, this only links). The role files are rewritten in place; the
originals are kept beside them as .proposed.
"""
import glob, json, os, subprocess, sys

M = os.path.dirname(os.path.abspath(__file__))
CLONE = os.path.join(M, "clone")
HOME = os.path.join(M, "home-probe")
FAN = os.path.join(HOME, "state/deligoez/cr/pr-1/fanout/1")
ENV = dict(os.environ, CR_HOME=HOME)

def run_probe(m):
    args = ["cr", "probe", "run", "1", "--kind", m["kind"]]
    if m["kind"] == "mutation":
        args += ["--patch", m["file"]]
    else:
        args += ["--test", m["file"], "--target", m.get("target", "")]
    if m.get("filter"):
        args += ["--filter", m["filter"]]
    for p in m.get("paths") or []:
        args += ["--path", p]
    r = subprocess.run(args, cwd=CLONE, env=ENV, capture_output=True, text=True)
    out = {}
    try:
        out = json.loads(r.stdout) if r.stdout.strip().startswith("{") else {}
    except json.JSONDecodeError:
        pass
    err = {}
    for line in r.stderr.splitlines():
        if line.startswith("{"):
            try:
                err = json.loads(line)
            except json.JSONDecodeError:
                pass
    return r.returncode, out, err, r.stderr[-600:]

if __name__ == "__main__":
    only = sys.argv[1:]  # optional manifest tags to (re)run
    results = []
    manifests = sorted(glob.glob(os.path.join(M, "probes-proposed", "*.ndjson")))
    for mf in manifests:
        tag = os.path.basename(mf)[:-7]
        if only and tag not in only:
            continue
        for line in open(mf):
            line = line.strip()
            if not line:
                continue
            try:
                m = json.loads(line)
            except json.JSONDecodeError as e:
                results.append({"tag": tag, "manifest": line[:200], "error": f"manifest not json: {e}"})
                continue
            if not os.path.isabs(m.get("file", "")) or not os.path.exists(m["file"]):
                results.append({"tag": tag, "manifest": m, "error": "input file missing"})
                continue
            code, out, err, tail = run_probe(m)
            entry = {"tag": tag, "manifest": m, "exit": code,
                     "probe": out.get("probe"), "result": out.get("result"), "establishes": out.get("establishes"),
                     "target": out.get("target"), "baseline": out.get("baseline"), "run": out.get("run"),
                     "reason": out.get("reason"), "error": err.get("error") or (tail if code else None)}
            results.append(entry)
            print(f"{tag} {m['kind']:8s} {m.get('record')} -> exit {code} {entry['probe']} {entry['result']} {entry['establishes']} {('ERR ' + str(entry['error'])[:120]) if entry['error'] else ''}")
    with open(os.path.join(M, "probes-run.ndjson"), "a") as fh:
        for e in results:
            fh.write(json.dumps(e) + "\n")

    # attach: record id -> first usable probe id
    usable = {}
    for e in results:
        rec = (e.get("manifest") or {}).get("record") if isinstance(e.get("manifest"), dict) else None
        if rec and e.get("probe") and e.get("result") not in (None, "error", "timeout") and rec not in usable:
            usable[rec] = e["probe"]
    attached = 0
    for rf in glob.glob(os.path.join(FAN, "*", "review-*.ndjson")):
        lines = [l for l in open(rf) if l.strip()]
        recs = [json.loads(l) for l in lines]
        changed = False
        for r in recs:
            if r.get("id") in usable and not r.get("probe"):
                r["probe"] = usable[r["id"]]; changed = True; attached += 1
        if changed:
            if not os.path.exists(rf + ".proposed"):
                os.rename(rf, rf + ".proposed")
            with open(rf, "w") as fh:
                for r in recs:
                    fh.write(json.dumps(r) + "\n")
    print(f"probes run: {len(results)}; usable: {len(usable)}; records linked: {attached}")
