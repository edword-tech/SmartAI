# SmartAI：GitHub 上传与维护指南

SmartAI 是统一异构算力管理平台，项目首页使用英文 README，便于国际开发者了解和参与。这里的“孵化”指项目早期建设阶段，不代表已经加入 CNCF、Apache 或其他基金会。

## 1. 选择仓库方式

当前本地仓库的 `origin` 指向 `https://github.com/edword-tech/SmartAI.git`，可将已提交的 SmartAI 改动推送到该仓库。

- **Fork 上游**：适合继续跟踪 SmartAI、保留 GitHub 上的派生关系，并向上游贡献。先在上游 GitHub 页面点击 Fork，再使用自己 Fork 的地址。
- **创建独立 SmartAI 仓库**：适合独立维护品牌和路线，但仍需保留来源与版权说明。新仓库先设为 Private，完成代码、历史和许可检查后再决定是否公开。

当前 checkout 没有找到仓库级 LICENSE，而且包含已有的硬件相关二进制。公开前应确认上游代码和这些文件的使用、修改及再分发条件。不要直接添加一份 MIT/Apache LICENSE 就认为它覆盖了所有上游内容。

## 2. 在 GitHub 创建仓库

登录 GitHub，点击右上角 **+ → New repository**：

- Repository name：`smartai`
- Description：`Unified heterogeneous compute management for GPU, NPU, and CPU workloads.`
- Visibility：准备阶段建议 Private。
- 独立空仓库：不要勾选初始化 README、.gitignore 或 License，本地已有 Git 历史。

如果选择 Fork，则跳过创建独立空仓库这一步。下文的 `YOUR_ACCOUNT` 替换为你的 GitHub 用户名或组织名。

## 3. 配置本机 Git 认证

先确认工具可用：

```bash
git --version
gh --version
```

安装 GitHub CLI 后，执行：

```bash
gh auth login
```

按提示选择 GitHub.com、HTTPS，并通过浏览器授权。不要把密码或 Token 写入仓库地址、脚本或 README。也可以使用已经配置好的 SSH 认证。

## 4. 检查将要上传的内容

```bash
cd /home/edword/smartai
git status --short
git remote -v
git diff --stat
```

本项目已有多项本地开发改动。提交前应确认这些改动都属于准备发布的版本。

以下内容不应上传：

- `unified-resource-manager/data/`：管理员密码、密码哈希、审计和运行数据。
- `.env`、云凭据、私钥、Token 和个人配置。
- `node_modules/`、`dist/`、日志和本地缓存。

`.gitignore` 只能排除尚未跟踪的文件，不能清除以前提交过的内容。公开仓库之前，还需检查历史中是否存在凭据、私有配置或不应分发的文件。若曾提交真实凭据，应先撤销或更换它们。

## 5. 添加自己的远程地址并提交

为保留当前上游地址，给自己的仓库增加一个独立远程名称 `smartai`。以下是操作说明，本次文档更新不会自动执行这些 Git 操作。

```bash
# 独立仓库用 smartai；Fork 则换成自己的 Fork 地址
git remote add smartai https://github.com/YOUR_ACCOUNT/smartai.git
git remote -v

# 完整上传当前平台版本：先确认所有本地改动都属于本次发布范围
git add .
# 如果只提交部分改动，请改用 git add <具体路径>，不要执行上面的全量添加

# 检查暂存区：确认没有遗漏功能文件，也没有包含敏感数据
git diff --cached --stat
git diff --cached
git status --short

git commit -m "Introduce SmartAI unified compute platform"
```

如 `smartai` 远程已经存在，用 `git remote get-url smartai` 核对；地址确实需要修改时再使用 `git remote set-url smartai <自己的地址>`。

### 上传到独立空仓库

```bash
# 将当前本地分支上传为远程 main，不需要改名当前开发分支
git push -u smartai HEAD:main
```

### 上传到 Fork

Fork 通常已有默认分支，先把当前改动上传为功能分支，再在 GitHub 发起 Pull Request：

```bash
git push -u smartai HEAD:codex/smartai-incubation
```

在 GitHub 的 **Compare & pull request** 中，将 base repository 选择为自己的 Fork，检查目标分支后创建 PR。不要误选上游仓库。

如果推送提示 non-fast-forward，先检查远程已有提交，通过分支和 PR 合并；不要直接使用 `--force`。

## 6. 日常如何管理

建议把 GitHub 当作“需求 → 修改 → 检查 → 合并 → 发布”的协作入口：

| 功能 | 用途 | 建议做法 |
| --- | --- | --- |
| Issues | 记录问题和需求 | 写明现象、复现步骤、环境和预期结果 |
| Labels | 分类 | `bug`、`enhancement`、`documentation`、`good first issue` |
| Projects | 看板 | Todo → In progress → In review → Done |
| Pull requests | 审查修改 | 每个 PR 聚焦一个问题，说明变化和验证结果 |
| Rulesets / branch protection | 保护主分支 | 条件允许时要求 PR、检查通过和审查；禁止强推 |
| Actions | 自动检查 | Go 测试、前端构建和语言覆盖检查 |
| Releases | 发布可复现版本 | 使用版本标签，附更新说明和已知限制 |

GitHub 的仓库类型和套餐会影响部分规则功能的可用性；以仓库 Settings 中实际提供的选项为准。单人维护时不要设置无法满足的强制审查人数。

日常改动示例（已有自己的远程 `main` 后）：

```bash
git fetch smartai
git switch -c codex/short-description smartai/main

# 修改代码并验证，再显式添加相关文件
git add <相关文件路径>
git commit -m "Describe the change"
git push -u smartai HEAD
```

然后在 GitHub 创建 PR，检查 diff 和测试结果后合并。新的功能分支应从更新后的 `smartai/main` 创建。

## 7. 建议的首轮孵化事项

1. 确认许可证、上游版权及二进制分发范围。
2. 完善英文首页、快速启动和兼容性说明。
3. 建立 CI；当前可运行 `go test ./...`、`npm run test:i18n` 和 `npm run build`。
4. 用 Issues 记录真实集群执行、硬件验证和账号生命周期等工作。
5. 首个可复现版本完成后发布预览版，并明确演示能力与真实执行能力的边界。

## 官方参考

- [上传已有本地仓库](https://docs.github.com/en/migrations/importing-source-code/using-the-command-line-to-import-source-code/adding-locally-hosted-code-to-github)
- [受保护分支](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches)
- [管理分支保护规则](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/managing-a-branch-protection-rule)
