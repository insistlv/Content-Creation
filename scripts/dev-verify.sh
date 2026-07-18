#!/usr/bin/env bash
#
# dev-verify.sh — 一键本地验证脚本
#
# 作用：自动发现 go / node / npm 工具链（即使没配好 PATH），然后按提交前门禁
#       依次执行「后端 vet + 测试 + 编译」和「前端 测试 + 构建」，最后汇总结果。
#
# 用法：
#   ./scripts/dev-verify.sh                 # 全量：后端 + 前端
#   ./scripts/dev-verify.sh --backend-only  # 只验证后端
#   ./scripts/dev-verify.sh --frontend-only # 只验证前端
#   ./scripts/dev-verify.sh --quick         # 后端只跑本轮改动相关测试（RateLimiter|WorkspaceDiscoveryTools）
#   ./scripts/dev-verify.sh --skip-build    # 跳过 go build 与 web build，仅跑测试
#
# 可用环境变量：
#   GO_TEST_ARGS   覆盖后端测试参数（默认 ./...）
#
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/.." && pwd)

# ---- 终端颜色（非 TTY 时自动降级为空）----
if [[ -t 1 ]]; then
	C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'; C_BLUE=$'\033[34m'; C_BOLD=$'\033[1m'; C_RESET=$'\033[0m'
else
	C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_BOLD=""; C_RESET=""
fi

log()  { printf '%s\n' "$*" >&2; }
info() { log "${C_BLUE}==>${C_RESET} $*"; }
ok()   { log "${C_GREEN}✓${C_RESET} $*"; }
warn() { log "${C_YELLOW}!${C_RESET} $*"; }
err()  { log "${C_RED}✗${C_RESET} $*"; }

# ---- 参数解析 ----
RUN_BACKEND=1
RUN_FRONTEND=1
QUICK=0
SKIP_BUILD=0
for arg in "$@"; do
	case "$arg" in
		--backend-only)  RUN_FRONTEND=0 ;;
		--frontend-only) RUN_BACKEND=0 ;;
		--quick)         QUICK=1 ;;
		--skip-build)    SKIP_BUILD=1 ;;
		-h|--help)
			sed -n '2,17p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
			exit 0
			;;
		*) err "未知参数：$arg（用 -h 查看帮助）"; exit 2 ;;
	esac
done

# ---- 自动补全常见工具链路径，最大化发现 go/node ----
augment_path() {
	local candidate
	for candidate in \
		/opt/homebrew/bin /opt/homebrew/sbin \
		/usr/local/bin /usr/local/sbin /usr/local/go/bin \
		"${HOME}/.local/go/bin" "${HOME}/.local/node/bin" \
		"${HOME}/go/bin" "${HOME}/.local/bin"; do
		if [[ -d "$candidate" ]]; then
			case ":${PATH}:" in
				*":${candidate}:"*) ;;
				*) PATH="${candidate}:${PATH}" ;;
			esac
		fi
	done
	# nvm（按当前 default 版本）
	if [[ -s "${HOME}/.nvm/nvm.sh" ]]; then
		# shellcheck disable=SC1091
		. "${HOME}/.nvm/nvm.sh" >/dev/null 2>&1 || true
	fi
	# asdf
	if [[ -s "${HOME}/.asdf/asdf.sh" ]]; then
		# shellcheck disable=SC1091
		. "${HOME}/.asdf/asdf.sh" >/dev/null 2>&1 || true
	fi
	export PATH
}
augment_path

# ---- 工具链检查 ----
MISSING=0
check_tool() {
	local name="$1" hint="$2"
	if command -v "$name" >/dev/null 2>&1; then
		ok "$name -> $(command -v "$name") ($("$name" version 2>/dev/null | head -1 || "$name" -v 2>/dev/null | head -1 || echo '?'))"
	else
		err "$name 未找到。安装建议：${hint}"
		MISSING=1
	fi
}

info "工具链检查（PATH 已自动补全 Homebrew/nvm/asdf 常见路径）"
if [[ $RUN_BACKEND -eq 1 ]]; then
	check_tool go "brew install go  或见 https://go.dev/dl/"
fi
if [[ $RUN_FRONTEND -eq 1 ]]; then
	check_tool node "brew install node  或见 https://nodejs.org/"
	check_tool npm  "随 Node.js 一并安装"
fi
if [[ $MISSING -ne 0 ]]; then
	err "缺少必要工具链，已中止。装好后重跑本脚本即可。"
	exit 1
fi

# ---- 执行步骤并记录结果 ----
FAILED=()
run_step() {
	local label="$1"; shift
	info "${C_BOLD}${label}${C_RESET}"
	if "$@"; then
		ok "${label} 通过"
	else
		err "${label} 失败"
		FAILED+=("$label")
	fi
}

if [[ $RUN_BACKEND -eq 1 ]]; then
	run_step "go vet ./..." bash -c "cd '${REPO_ROOT}' && go vet ./..."
	if [[ $QUICK -eq 1 ]]; then
		run_step "go test (本轮改动)" bash -c "cd '${REPO_ROOT}' && go test ./internal/app -run 'RateLimiter|WorkspaceDiscoveryTools' -count=1"
	else
		run_step "go test ${GO_TEST_ARGS:-./...}" bash -c "cd '${REPO_ROOT}' && go test ${GO_TEST_ARGS:-./...} -count=1"
	fi
	if [[ $SKIP_BUILD -eq 0 ]]; then
		run_step "go build ./cmd/server" bash -c "cd '${REPO_ROOT}' && go build -o /dev/null ./cmd/server"
	fi
fi

if [[ $RUN_FRONTEND -eq 1 ]]; then
	if [[ ! -d "${REPO_ROOT}/web/node_modules" ]]; then
		run_step "npm install (web)" bash -c "cd '${REPO_ROOT}/web' && npm install"
	fi
	run_step "npm test (web)" bash -c "cd '${REPO_ROOT}/web' && npm test"
	if [[ $SKIP_BUILD -eq 0 ]]; then
		run_step "npm run build (web)" bash -c "cd '${REPO_ROOT}/web' && npm run build"
	fi
fi

# ---- 汇总 ----
log ""
if [[ ${#FAILED[@]} -eq 0 ]]; then
	ok "${C_BOLD}全部验证通过。${C_RESET}"
	exit 0
fi
err "${C_BOLD}以下步骤失败：${C_RESET}"
for f in "${FAILED[@]}"; do
	log "  ${C_RED}- ${f}${C_RESET}"
done
exit 1
