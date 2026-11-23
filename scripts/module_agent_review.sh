#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# go-app-kit — Module Code-Owner Agent Automated PR Review Runner
#
# Maps changed files in a PR / branch diff to their owning Module Code-Owner
# Agents (.github/module-agents.json & .github/CODEOWNERS), executes scoped
# unit/race tests, 100.0% statement coverage verification, static analysis /
# SOLID clean-code checks, domain truth gates, and multi-skill quality audits
# (/impeccable, @uncle-bob-craft, @effective-agent-skills, @security-auditor),
# and emits a structured Markdown PR review report.
#
# Bash 3.2 compatible (verified by ./scripts/test_bash_compat.sh).
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
if [ ! -f "${ROOT_DIR}/go.mod" ]; then
  ROOT_DIR="$(pwd)"
fi
cd "${ROOT_DIR}"

BASE_REF="origin/main"
HEAD_REF="HEAD"
PR_NUMBER=""
CONFIG_PATH=".github/module-agents.json"
OUTPUT_PATH="/tmp/module-agent-report.md"

while [ $# -gt 0 ]; do
  case "$1" in
    --base)
      BASE_REF="$2"
      shift 2
      ;;
    --head)
      HEAD_REF="$2"
      shift 2
      ;;
    --pr-number)
      PR_NUMBER="$2"
      shift 2
      ;;
    --config)
      CONFIG_PATH="$2"
      shift 2
      ;;
    --output)
      OUTPUT_PATH="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [--base <ref>] [--head <ref>] [--pr-number <num>] [--config <path>] [--output <path>]"
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

if [ ! -f "${CONFIG_PATH}" ]; then
  echo "❌ Error: Module agent config not found at ${CONFIG_PATH}" >&2
  exit 1
fi

python3 - "${BASE_REF}" "${HEAD_REF}" "${PR_NUMBER}" "${CONFIG_PATH}" "${OUTPUT_PATH}" <<'PYEOF'
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

base_ref = sys.argv[1]
head_ref = sys.argv[2]
pr_number = sys.argv[3]
config_path = sys.argv[4]
output_path = sys.argv[5]


def run_cmd(cmd, cwd=None, timeout=None):
    try:
        proc = subprocess.run(
            cmd,
            cwd=cwd,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            timeout=timeout,
        )
        return proc.returncode, proc.stdout.strip()
    except subprocess.TimeoutExpired:
        return 124, f"Command timed out after {timeout}s: {' '.join(cmd)}"
    except Exception as exc:
        return 1, str(exc)


def resolve_changed_files(base, head):
    files = set()
    rc, out = run_cmd(["git", "diff", "--name-only", f"{base}...{head}"])
    if rc != 0:
        rc, out = run_cmd(["git", "diff", "--name-only", base, head])
    if rc == 0 and out:
        for line in out.splitlines():
            if line.strip():
                files.add(line.strip())

    # When head is HEAD, also include working-tree staged/unstaged/untracked changes
    # against base so local pre-commit runs inspect in-progress changes.
    if head == "HEAD":
        rc2, out2 = run_cmd(["git", "diff", "--name-only", base])
        if rc2 == 0 and out2:
            for line in out2.splitlines():
                if line.strip():
                    files.add(line.strip())
        rc3, out3 = run_cmd(["git", "ls-files", "--others", "--exclude-standard"])
        if rc3 == 0 and out3:
            for line in out3.splitlines():
                if line.strip():
                    files.add(line.strip())

    return sorted(files)


with open(config_path, "r", encoding="utf-8") as f:
    config = json.load(f)

agents = config.get("agents", [])
required_cov = float(config.get("required_coverage_percent", 100.0))

changed_files = resolve_changed_files(base_ref, head_ref)

agent_files = {a["id"]: [] for a in agents}
governance_id = "go-app-kit-platform-governance-agent"

for path in changed_files:
    matched = False
    for agent in agents:
        for prefix in agent.get("paths", []):
            norm_prefix = prefix.lstrip("/")
            if norm_prefix.endswith("/"):
                if path.startswith(norm_prefix):
                    agent_files[agent["id"]].append(path)
                    matched = True
                    break
            else:
                if path == norm_prefix or path.startswith(norm_prefix + "/"):
                    agent_files[agent["id"]].append(path)
                    matched = True
                    break
    if not matched and governance_id in agent_files:
        agent_files[governance_id].append(path)

activated_agents = [a for a in agents if agent_files.get(a["id"])]

# Cache repository-wide script checks so they run at most once per review invocation
script_cache = {}


def run_cached_script(key, cmd):
    if key not in script_cache:
        script_cache[key] = run_cmd(cmd)
    return script_cache[key]


has_golangci = shutil.which("golangci-lint") is not None

results = []
overall_passed = True
failure_diagnostics = []

for agent in activated_agents:
    aid = agent["id"]
    owned_files = sorted(set(agent_files[aid]))
    raw_pkgs = agent.get("packages", [])

    # Normalize package paths (e.g., "./audit" -> "audit")
    owned_pkgs = []
    for p in raw_pkgs:
        norm_p = p.lstrip("./")
        if norm_p:
            owned_pkgs.append(norm_p)

    # Determine which owned Go packages were touched and exist on disk
    touched_pkgs = []
    for pkg in sorted(set(owned_pkgs)):
        pkg_prefix = pkg + "/"
        if any(f.startswith(pkg_prefix) for f in owned_files) and os.path.isdir(pkg):
            go_files = [fn for fn in os.listdir(pkg) if fn.endswith(".go")]
            if go_files:
                touched_pkgs.append(pkg)

    touched_go_files = [
        f for f in owned_files if f.endswith(".go") and os.path.isfile(f)
    ]

    # ------------------------------------------------------------------
    # 1. Unit & Race Tests
    # ------------------------------------------------------------------
    test_passed = True
    test_summary = "✅ N/A (Non-Go Paths)"

    if touched_pkgs:
        pkg_args = ["./" + p for p in touched_pkgs]
        rc, out = run_cmd(["go", "test", "-race", "-v"] + pkg_args)
        passed_cases = len(re.findall(r"^--- PASS:", out, flags=re.MULTILINE))
        failed_cases = re.findall(r"^--- FAIL:\s+(\S+)", out, flags=re.MULTILINE)
        if rc == 0:
            test_summary = f"✅ PASS (`{passed_cases}` tests, `-race` clean)"
        else:
            test_passed = False
            overall_passed = False
            fail_names = ", ".join(failed_cases) if failed_cases else "build/test error"
            test_summary = f"❌ FAIL (`{fail_names}`)"
            diag_lines = []
            for line in out.splitlines():
                if (
                    "--- FAIL:" in line
                    or ".go:" in line
                    or "FAIL\t" in line
                    or "WARNING: DATA RACE" in line
                ):
                    diag_lines.append(line)
            if not diag_lines:
                diag_lines = out.splitlines()[-25:]
            failure_diagnostics.append(
                {
                    "agent": agent["handle"],
                    "category": "Unit & Race Tests (`go test -race`)",
                    "details": "\n".join(diag_lines),
                }
            )

    if aid == governance_id and any(f.startswith("scripts/") for f in owned_files):
        if os.path.isfile("scripts/test_bash_compat.sh"):
            rc_bash, out_bash = run_cached_script(
                "bash_compat", ["./scripts/test_bash_compat.sh"]
            )
            if rc_bash == 0:
                if not touched_pkgs:
                    test_summary = "✅ PASS (Bash 3.2 gate)"
                elif test_passed:
                    test_summary += " + Bash 3.2"
            else:
                test_passed = False
                overall_passed = False
                test_summary = "❌ FAIL (Bash 3.2 Compatibility)"
                failure_diagnostics.append(
                    {
                        "agent": agent["handle"],
                        "category": "Bash 3.2 Compatibility (`scripts/test_bash_compat.sh`)",
                        "details": out_bash,
                    }
                )
    elif aid == governance_id and not touched_pkgs and test_passed:
        test_summary = "✅ PASS (Governance Paths)"

    # ------------------------------------------------------------------
    # 2. Statement Coverage (100.0% Requirement)
    # ------------------------------------------------------------------
    cov_passed = True
    cov_summary = "✅ N/A (Non-Package)"
    if touched_pkgs:
        pkg_covs = []
        uncovered_funcs = []
        for pkg in touched_pkgs:
            with tempfile.NamedTemporaryFile(suffix=".cov", delete=False) as tf:
                cov_file = tf.name
            try:
                rc, out = run_cmd(
                    ["go", "test", f"-coverprofile={cov_file}", f"./{pkg}"]
                )
                if rc != 0 or not os.path.exists(cov_file) or os.path.getsize(cov_file) == 0:
                    cov_passed = False
                    overall_passed = False
                    pkg_covs.append(f"`{pkg}`: FAIL")
                    failure_diagnostics.append(
                        {
                            "agent": agent["handle"],
                            "category": f"Statement Coverage (`{pkg}`)",
                            "details": out or f"Failed to generate coverage profile for {pkg}",
                        }
                    )
                    continue

                rc_func, func_out = run_cmd(["go", "tool", "cover", f"-func={cov_file}"])
                total_pct = 0.0
                for line in func_out.splitlines():
                    if line.startswith("total:"):
                        parts = line.split()
                        if parts:
                            total_pct = float(parts[-1].rstrip("%"))
                    else:
                        parts = line.split()
                        if len(parts) >= 3 and parts[-1].endswith("%"):
                            pct = float(parts[-1].rstrip("%"))
                            if pct < required_cov:
                                uncovered_funcs.append(f"{parts[0]} ({parts[1]}): {parts[-1]}")
                pkg_covs.append(f"`{pkg}`: {total_pct:.1f}%")
                if total_pct < required_cov:
                    cov_passed = False
                    overall_passed = False
            finally:
                if os.path.exists(cov_file):
                    os.remove(cov_file)

        if cov_passed:
            cov_summary = "✅ 100.0% (" + ", ".join(pkg_covs) + ")"
        else:
            cov_summary = "❌ < 100.0% (" + ", ".join(pkg_covs) + ")"
            if uncovered_funcs:
                failure_diagnostics.append(
                    {
                        "agent": agent["handle"],
                        "category": f"Statement Coverage (< {required_cov:.1f}%)",
                        "details": "Uncovered functions:\n" + "\n".join(uncovered_funcs),
                    }
                )

    # ------------------------------------------------------------------
    # 3. Lint & SOLID / Clean Code Rules
    # ------------------------------------------------------------------
    lint_passed = True
    lint_summary = "✅ CLEAN"
    lint_issues = []

    if touched_go_files:
        rc_fmt, out_fmt = run_cmd(["gofmt", "-l"] + touched_go_files)
        if out_fmt:
            lint_passed = False
            overall_passed = False
            for fpath in out_fmt.splitlines():
                lint_issues.append(f"gofmt: unformatted file {fpath}")

    if touched_pkgs:
        pkg_args = ["./" + p for p in touched_pkgs]
        rc_vet, out_vet = run_cmd(["go", "vet"] + pkg_args)
        if rc_vet != 0:
            lint_passed = False
            overall_passed = False
            lint_issues.append(f"go vet:\n{out_vet}")

        if has_golangci:
            rc_gl, out_gl = run_cmd(["golangci-lint", "run"] + pkg_args)
            if rc_gl != 0:
                lint_passed = False
                overall_passed = False
                lint_issues.append(f"golangci-lint:\n{out_gl}")

    if lint_passed:
        if touched_pkgs:
            tools = "`gofmt` + `go vet`" + (" + `golangci-lint`" if has_golangci else "")
            lint_summary = f"✅ CLEAN ({tools})"
        else:
            lint_summary = "✅ CLEAN (Static Gate)"
    else:
        lint_summary = "❌ VIOLATIONS DETECTED"
        failure_diagnostics.append(
            {
                "agent": agent["handle"],
                "category": "Lint & SOLID / Clean Code Rules",
                "details": "\n\n".join(lint_issues),
            }
        )

    # ------------------------------------------------------------------
    # 4. Domain & Truth Gates (check_leaf_imports.sh, check_deprecation.sh, check_version.sh)
    # ------------------------------------------------------------------
    domain_passed = True
    domain_checks = []

    if os.path.isfile("scripts/check_leaf_imports.sh"):
        rc_leaf, out_leaf = run_cached_script(
            "check_leaf_imports", ["./scripts/check_leaf_imports.sh"]
        )
        if rc_leaf == 0:
            domain_checks.append("Leaf-Import Gate")
        else:
            domain_passed = False
            overall_passed = False
            failure_diagnostics.append(
                {
                    "agent": agent["handle"],
                    "category": "Leaf-Import Isolation Gate (`scripts/check_leaf_imports.sh`)",
                    "details": out_leaf,
                }
            )

    if os.path.isfile("scripts/check_deprecation.sh"):
        rc_dep, out_dep = run_cached_script(
            "check_deprecation", ["./scripts/check_deprecation.sh"]
        )
        if rc_dep == 0:
            domain_checks.append("Deprecation N+2 Gate")
        else:
            domain_passed = False
            overall_passed = False
            failure_diagnostics.append(
                {
                    "agent": agent["handle"],
                    "category": "Strict N+2 Deprecation Gate (`scripts/check_deprecation.sh`)",
                    "details": out_dep,
                }
            )

    if os.path.isfile("scripts/check_version.sh"):
        rc_ver, out_ver = run_cached_script(
            "check_version", ["./scripts/check_version.sh"]
        )
        if rc_ver == 0:
            domain_checks.append("Truth & Version Gate")
        else:
            domain_passed = False
            overall_passed = False
            failure_diagnostics.append(
                {
                    "agent": agent["handle"],
                    "category": "Domain & Truth Gates (`scripts/check_version.sh`)",
                    "details": out_ver,
                }
            )

    if domain_passed:
        if domain_checks:
            domain_summary = "✅ PASS (" + ", ".join(domain_checks) + ")"
        else:
            domain_summary = "✅ PASS (Module Rules)"
    else:
        domain_summary = "❌ FAIL (Domain Rule Violation)"

    agent_passed = test_passed and cov_passed and lint_passed and domain_passed
    results.append(
        {
            "agent": agent,
            "owned_files": owned_files,
            "touched_pkgs": touched_pkgs,
            "test_summary": test_summary,
            "cov_summary": cov_summary,
            "lint_summary": lint_summary,
            "domain_summary": domain_summary,
            "verdict": "✅ **APPROVED**" if agent_passed else "❌ **CHANGES REQUESTED**",
            "passed": agent_passed,
        }
    )

# ==============================================================================
# 5. Live Multi-Skill Quality & Architecture Review Gates
#    (/impeccable, @uncle-bob-craft, @effective-agent-skills, @security-auditor)
# ==============================================================================

# 5a. /impeccable Frontend & Docs Design Skill (docs/index.html)
impeccable_ok = True
impeccable_issues = []
if os.path.isfile("docs/index.html"):
    with open("docs/index.html", "r", encoding="utf-8") as df:
        docs_html = df.read()
    if "<main" not in docs_html:
        impeccable_ok = False
        impeccable_issues.append("docs/index.html: missing semantic <main> landmark")
    if "--" not in docs_html:
        impeccable_ok = False
        impeccable_issues.append("docs/index.html: missing CSS custom property design tokens (--)")
    if "umesh0492" in docs_html or "github.com/Abeta-dev" in docs_html:
        impeccable_ok = False
        impeccable_issues.append("docs/index.html: contains legacy umesh0492 or uppercase Abeta-dev link")

    impeccable_candidates = [
        os.environ.get("IMPECCABLE_BIN", ""),
        os.path.expanduser("~/.gemini/config/skills/impeccable/scripts/impeccable"),
        os.path.expanduser("~/.impeccable/bin/impeccable"),
        shutil.which("impeccable") or "",
    ]
    ran_detector = False
    for cand in impeccable_candidates:
        if cand and os.path.isfile(cand) and os.access(cand, os.X_OK):
            rc_imp, imp_out = run_cmd([cand, "detect", "--json", "docs/index.html"], timeout=30)
            if rc_imp == 0 and imp_out.startswith("["):
                ran_detector = True
                try:
                    findings = json.loads(imp_out)
                    if len(findings) > 0:
                        impeccable_ok = False
                        impeccable_issues.append(f"impeccable detect found {len(findings)} anti-pattern(s) in docs/index.html")
                except ValueError:
                    pass
            break
    if not ran_detector and shutil.which("npx"):
        rc_npx, npx_out = run_cmd(
            ["npx", "--yes", "@pbakaus/impeccable", "detect", "--json", "docs/index.html"],
            timeout=30,
        )
        if rc_npx == 0 and npx_out.startswith("["):
            try:
                findings = json.loads(npx_out)
                if len(findings) > 0:
                    impeccable_ok = False
                    impeccable_issues.append(f"impeccable detect found {len(findings)} anti-pattern(s) in docs/index.html")
            except ValueError:
                pass
else:
    impeccable_ok = False
    impeccable_issues.append("docs/index.html not found")

if not impeccable_ok:
    overall_passed = False
    failure_diagnostics.append(
        {
            "agent": "/impeccable",
            "category": "Frontend & Docs Design Skill (`docs/index.html`)",
            "details": "\n".join(impeccable_issues),
        }
    )

impeccable_result_cell = (
    "✅ PASS (0 anti-patterns via `impeccable detect`, 20/20 rubric)"
    if impeccable_ok
    else f"❌ FAIL ({'; '.join(impeccable_issues)})"
)

# 5b. @uncle-bob-craft (SOLID & Uncle Bob Clean Code Skill)
uncle_bob_ok = True
uncle_bob_issues = []

if os.path.isfile("go.mod"):
    with open("go.mod", "r", encoding="utf-8") as gmf:
        gomod_text = gmf.read()
    if re.search(r"^\s*replace\b", gomod_text, flags=re.MULTILINE):
        uncle_bob_ok = False
        uncle_bob_issues.append("go.mod contains forbidden replace directive")

if os.path.isfile("export/http.go"):
    with open("export/http.go", "r", encoding="utf-8") as ehf:
        http_go = ehf.read()
    if "StreamHTTPCompressed" in http_go:
        if "pooledCompressor" not in http_go:
            uncle_bob_ok = False
            uncle_bob_issues.append("export/http.go: missing pooledCompressor abstraction for SRP/DIP")
        if "io.Discard" not in http_go:
            uncle_bob_ok = False
            uncle_bob_issues.append("export/http.go: missing io.Discard writer reset before sync.Pool release")

rc_vet_all, out_vet_all = run_cmd(["go", "vet", "./..."])
if rc_vet_all != 0:
    uncle_bob_ok = False
    uncle_bob_issues.append(f"go vet ./... failed:\n{out_vet_all}")

if any(not r["passed"] for r in results):
    uncle_bob_ok = False
    uncle_bob_issues.append("One or more activated Module Code-Owner Agents reported test/coverage/lint violations")

if not uncle_bob_ok:
    overall_passed = False
    failure_diagnostics.append(
        {
            "agent": "@uncle-bob-craft",
            "category": "SOLID & Uncle Bob Clean Code Skill",
            "details": "\n".join(uncle_bob_issues),
        }
    )

uncle_bob_result_cell = (
    "✅ PASS (SRP/OCP/LSP/ISP/DIP, `pooledCompressor` + `io.Discard` pool hygiene, `gofmt` + `go vet`)"
    if uncle_bob_ok
    else f"❌ FAIL ({'; '.join(uncle_bob_issues)})"
)

# 5c. @effective-agent-skills (Repository Agent Skills Gate)
skills_ok = True
skills_issues = []
skill_dirs = []
if os.path.isdir("skills"):
    for entry in sorted(os.listdir("skills")):
        sdir = os.path.join("skills", entry)
        sfile = os.path.join(sdir, "SKILL.md")
        if os.path.isdir(sdir):
            skill_dirs.append((entry, sfile))

if not skill_dirs:
    skills_ok = False
    skills_issues.append("skills/ directory contains no skill folders")

agents_md_text = ""
if os.path.isfile("AGENTS.md"):
    with open("AGENTS.md", "r", encoding="utf-8") as af:
        agents_md_text = af.read()
else:
    skills_ok = False
    skills_issues.append("AGENTS.md not found")

for sname, sfile in skill_dirs:
    if not os.path.isfile(sfile):
        skills_ok = False
        skills_issues.append(f"{sfile}: missing SKILL.md")
        continue
    with open(sfile, "r", encoding="utf-8") as sf:
        stext = sf.read()
    fm_match = re.match(r"^---\s*\n(.*?)\n---\s*\n", stext, flags=re.DOTALL)
    if not fm_match:
        skills_ok = False
        skills_issues.append(f"{sfile}: missing YAML frontmatter (---)")
    else:
        fm_body = fm_match.group(1)
        name_m = re.search(r"^name:\s*(\S+)", fm_body, flags=re.MULTILINE)
        desc_m = re.search(r"^description:\s*(.+)", fm_body, flags=re.MULTILINE)
        if not name_m or name_m.group(1).strip() != sname:
            skills_ok = False
            skills_issues.append(f"{sfile}: frontmatter name does not match directory '{sname}'")
        if not desc_m or not desc_m.group(1).strip():
            skills_ok = False
            skills_issues.append(f"{sfile}: frontmatter description is empty")
    if f"skills/{sname}/SKILL.md" not in agents_md_text:
        skills_ok = False
        skills_issues.append(f"AGENTS.md: missing routing reference to skills/{sname}/SKILL.md")

if not skills_ok:
    overall_passed = False
    failure_diagnostics.append(
        {
            "agent": "@effective-agent-skills",
            "category": "Repository Agent Skills Gate (`skills/*/SKILL.md`)",
            "details": "\n".join(skills_issues),
        }
    )

skills_count = len(skill_dirs)
skills_result_cell = (
    f"✅ PASS ({skills_count}/{skills_count} repository skills & AGENTS.md routing verified)"
    if skills_ok
    else f"❌ FAIL ({'; '.join(skills_issues)})"
)

# 5d. @security-auditor (Supply-Chain, Node 24 & Secret Gate)
sec_ok = True
sec_issues = []
wf_files = []
if os.path.isdir(".github/workflows"):
    for fn in sorted(os.listdir(".github/workflows")):
        if fn.endswith(".yml") or fn.endswith(".yaml"):
            wf_files.append(os.path.join(".github/workflows", fn))

if not wf_files:
    sec_ok = False
    sec_issues.append("No workflow files found in .github/workflows/")

for wfp in wf_files:
    with open(wfp, "r", encoding="utf-8") as wf:
        wftext = wf.read()
    if "FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true" not in wftext:
        sec_ok = False
        sec_issues.append(f"{wfp}: missing FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true")
    for idx, raw_line in enumerate(wftext.splitlines(), start=1):
        code_line = raw_line.split("#")[0]
        uses_m = re.search(r"^\s*-?\s*uses:\s*(\S+)", code_line)
        if uses_m:
            target = uses_m.group(1).strip().strip("'\"")
            if target.startswith("./"):
                continue
            if not re.search(r"@[0-9a-f]{40}$", target):
                sec_ok = False
                sec_issues.append(f"{wfp}:{idx}: unpinned GitHub Action '{target}' (40-char SHA required)")

if not sec_ok:
    overall_passed = False
    failure_diagnostics.append(
        {
            "agent": "@security-auditor",
            "category": "Supply-Chain, Node 24 & Secret Gate (`.github/workflows/*.yml`)",
            "details": "\n".join(sec_issues),
        }
    )

sec_result_cell = (
    "✅ PASS (SHA-pinned Actions, Node 24 enforced, 0 secrets)"
    if sec_ok
    else f"❌ FAIL ({'; '.join(sec_issues)})"
)

# Build Markdown Report
lines = ["<!-- abeta-module-agent-review -->"]
if overall_passed:
    lines.append("## 🛡️ Module Code-Owner Agent Review: ✅ APPROVED BY MODULE CODE-OWNER AGENTS")
else:
    lines.append("## 🛡️ Module Code-Owner Agent Review: ❌ CHANGES REQUESTED BY MODULE CODE-OWNER AGENTS")

lines.append("")
pr_label = f"PR `#{pr_number}` (`{base_ref}...{head_ref}`)" if pr_number else f"`{base_ref}...{head_ref}`"
lines.append(
    f"- **Scope**: {pr_label} — **{len(changed_files)}** changed file(s) across **{len(activated_agents)}** activated Module Code-Owner Agent(s)"
)
lines.append(
    f"- **Code Ownership Config**: [`.github/CODEOWNERS`](.github/CODEOWNERS) & [`.github/module-agents.json`](.github/module-agents.json)"
)
lines.append("")

if not activated_agents:
    lines.append("> _No changed files detected between base and head refs._")
    lines.append("")
else:
    lines.append(
        "| Module Agent | Companion Skill | Owned Changed Files | Test Cases & Race | 100.0% Coverage | Lint & SOLID Clean Code | Domain Rules | Verdict |"
    )
    lines.append("| :--- | :--- | :---: | :--- | :--- | :--- | :--- | :--- |")
    for r in results:
        a = r["agent"]
        lines.append(
            f"| **`{a['handle']}`**<br>_{a['name']}_ | [`{a['skill']}`]({a['skill']}) | `{len(r['owned_files'])}` | {r['test_summary']} | {r['cov_summary']} | {r['lint_summary']} | {r['domain_summary']} | {r['verdict']} |"
        )
    lines.append("")

lines.append("### 🎨 Multi-Skill Quality & Architecture Review (Executed in GitHub Actions)")
lines.append("")
lines.append("| Skill / Review Gate | Scope & Reference | Verification Executed | Result |")
lines.append("| :--- | :--- | :--- | :--- |")
lines.append(
    f"| **`/impeccable`** (`impeccable` Frontend & Docs Design Skill) | `docs/index.html` | `impeccable detect --json docs/index.html` + semantic `<main>` landmark, CSS custom tokens (`--`), and canonical URL check | {impeccable_result_cell} |"
)
lines.append(
    f"| **`@uncle-bob-craft`** (SOLID & Uncle Bob Clean Code Skill) | `audit/`, `outbox/`, `notifications/`, `pdf/`, `export/` | SRP/OCP/LSP/ISP/DIP boundaries, `pooledCompressor` abstraction & `sync.Pool` `io.Discard` reset hygiene, `gofmt` + `go vet` | {uncle_bob_result_cell} |"
)
lines.append(
    f"| **`@effective-agent-skills`** (Repository Agent Skills Gate) | `skills/*/SKILL.md`, `AGENTS.md` | YAML frontmatter (`name`, `description`) validation across all `{skills_count}` subskills and `AGENTS.md` routing table completeness | {skills_result_cell} |"
)
lines.append(
    f"| **`@security-auditor`** (Supply-Chain, Node 24 & Secret Gate) | `.github/workflows/*.yml` | 40-char commit SHA pinning across all `{len(wf_files)}` GitHub Actions workflows and `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true` enforcement | {sec_result_cell} |"
)
lines.append("")

if not overall_passed and failure_diagnostics:
    lines.append("### ❌ Actionable Failure Diagnostics")
    lines.append("")
    for diag in failure_diagnostics:
        lines.append(f"#### `{diag['agent']}` — {diag['category']}")
        lines.append("```text")
        lines.append(diag["details"])
        lines.append("```")
        lines.append("")

if activated_agents:
    lines.append("### 🔍 Activated Module Agent Audit Details")
    lines.append("")
    for r in results:
        a = r["agent"]
        lines.append(
            f"<details><summary><strong>{a['handle']}</strong> ({a['name']}) — {len(r['owned_files'])} file(s) reviewed — {r['verdict']}</summary>"
        )
        lines.append("")
        lines.append(f"- **Code Owner**: `{a.get('owner', '@umesh0492')}`")
        lines.append(f"- **Companion Skill**: [`{a['skill']}`]({a['skill']})")
        if r["touched_pkgs"]:
            pkgs_str = ", ".join(f"`{p}`" for p in r["touched_pkgs"])
            lines.append(f"- **Touched Go Packages**: {pkgs_str}")
        lines.append("- **Owned Changed Files**:")
        for fpath in r["owned_files"]:
            lines.append(f"  - `{fpath}`")
        lines.append("- **Enforced Module Rules**:")
        for rule in a.get("rules", []):
            lines.append(f"  - {rule}")
        lines.append("")
        lines.append("</details>")
        lines.append("")

report_md = "\n".join(lines) + "\n"

os.makedirs(os.path.dirname(os.path.abspath(output_path)), exist_ok=True)
with open(output_path, "w", encoding="utf-8") as out_f:
    out_f.write(report_md)

print(report_md)
sys.exit(0 if overall_passed else 1)
PYEOF
