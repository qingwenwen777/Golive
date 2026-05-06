# 仓库清理建议

这份清单只列出建议清理项。删除前先确认本地没有正在使用的浏览器会话或调试
进程。

## 建议从 git 移除

| 路径 | 当前状态 | 原因 |
| ---- | -------- | ---- |
| `golive-web/.chrome-analytics-check/` | 已被 git 跟踪，321 个文件，约 18.6 MB | Playwright/浏览器用户数据目录，包含缓存、历史、扩展和本地状态，不属于源码 |
| `golive-web/.edge-history-check/` | 已被 git 跟踪，536 个文件，约 26.9 MB | Edge 用户数据目录，不属于源码 |
| `golive-web/.edge-history-check-2/` | 已被 git 跟踪，343 个文件，约 18.2 MB | 同上 |
| `golive-web/.edge-history-check-3/` | 已被 git 跟踪，374 个文件，约 19.6 MB | 同上 |
| `golive-web/coverage/` | 已被 git 跟踪，25 个文件，约 0.37 MB | Vitest 覆盖率 HTML/JSON 产物，应由 CI 或本地测试生成 |

建议命令：

```bash
git rm -r --cached -- golive-web/.chrome-analytics-check golive-web/.edge-history-check golive-web/.edge-history-check-2 golive-web/.edge-history-check-3 golive-web/coverage
rm -rf golive-web/.chrome-analytics-check golive-web/.edge-history-check golive-web/.edge-history-check-2 golive-web/.edge-history-check-3 golive-web/coverage
git status --short
```

Windows PowerShell 等价命令：

```powershell
git rm -r --cached -- golive-web/.chrome-analytics-check golive-web/.edge-history-check golive-web/.edge-history-check-2 golive-web/.edge-history-check-3 golive-web/coverage
Remove-Item -Recurse -Force -LiteralPath golive-web/.chrome-analytics-check, golive-web/.edge-history-check, golive-web/.edge-history-check-2, golive-web/.edge-history-check-3, golive-web/coverage
git status --short
```

## 可直接删除的本地忽略文件

这些已经被 `.gitignore` 忽略，删除不会影响源码：

| 路径/模式 | 原因 |
| --------- | ---- |
| `golive-web/dist/` | 前端构建产物 |
| `golive-web/node_modules/` | 依赖目录 |
| `golive-web/*.log`、`golive-web/*.err.log`、`golive-web/*.out.log` | Vite/preview 调试日志 |
| `golive-web/.vite-codex.log` | 本地调试日志 |

清理命令：

```powershell
Remove-Item -Recurse -Force -LiteralPath golive-web/dist -ErrorAction SilentlyContinue
Remove-Item -Recurse -Force -LiteralPath golive-web/node_modules -ErrorAction SilentlyContinue
Remove-Item -Force -LiteralPath golive-web/*.log -ErrorAction SilentlyContinue
```

## 已补充的忽略规则

`golive-web/.gitignore` 和 `golive-web/.dockerignore` 已补充：

- `coverage`
- `.chrome-analytics-check*`
- `.edge-history-check*`
- `preview-*.log`
- `preview-*.err.log`
- `preview-*.out.log`
