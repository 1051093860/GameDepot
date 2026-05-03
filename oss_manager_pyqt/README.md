# OSS Manager PyQt

一个独立的 Python + PyQt OSS 管理器，不依赖旧的 Go Core，也不调用 `gamedepot.exe`。它会**直接读取 Go 版 GameDepot 已创建的 `.gamedepot/config.yaml`**，再直接扫描 Git 历史里的 pointer ref 文件，检查 OSS/blob 是否存在，并支持删除旧 blob、仅保留当前版本、提取指定历史版本到本地。

## 适用仓库结构

默认兼容 GameDepot pointer refs 结构：

```text
ProjectRoot/
  .gamedepot/config.yaml              # Go 创建的配置文件，本工具直接读取
  Content/...真实资产工作区，可存在也可缺失
  depot/refs/Content/.../*.gdref      # Git 跟踪的 pointer ref
```

左侧文件树来自 Git 历史中的 `depot/refs/**/*.gdref`，所以：

- HEAD 里还存在的文件会显示；
- 工作区文件不存在但 ref 还在 HEAD 的文件会显示；
- HEAD 已删除、但历史 commit 中出现过的文件也会显示。

`*.gdref` 可以是 JSON/YAML/简单 key-value，只要里面能提取到：

- `sha256` / `sha` / `hash` / `object_sha256` / `blobHash` 等；
- 可选 `key` / `oss_key` / `objectKey` / `blob_key` / `remoteKey` 等；
- 可选 `size` / `bytes` / `sizeBytes` 等。

如果 ref 里没有显式 `key`，工具会按配置中的 `key_template` 或 `key_prefix` 从 sha256 推导 OSS key。

## 安装

```bash
cd oss_manager_pyqt
python -m venv .venv
.venv\Scripts\activate  # Windows
# source .venv/bin/activate  # macOS/Linux
pip install -e .[aliyun]
```

如果只用本地 blob 目录测试：

```bash
pip install -e .
```

S3 / MinIO / COS / OBS 兼容模式：

```bash
pip install -e .[s3]
```

## 使用

```bash
program .
# 或
oss-manager .
# 或
python -m oss_manager .
```

## 配置读取规则

现在默认读取：

```text
.gamedepot/config.yaml
```

它会尝试兼容多种 Go 配置结构，例如下面这些字段名都可以被识别：

```yaml
storage:
  provider: aliyun
  endpoint: https://oss-cn-shenzhen.aliyuncs.com
  bucket: your-bucket-name
  access_key_id_env: ALIYUN_OSS_ACCESS_KEY_ID
  access_key_secret_env: ALIYUN_OSS_ACCESS_KEY_SECRET
  key_prefix: blobs

ref_root: depot/refs
ref_suffix: .gdref
```

也兼容类似：

```yaml
objectStore:
  type: s3-compatible
  endpointURL: http://127.0.0.1:9000
  bucketName: gamedepot
  accessKeyId: minioadmin
  accessKeySecret: minioadmin
  keyTemplate: blobs/{sha256}

pointerRefs:
  root: depot/refs
  suffix: .gdref
```

以及本地对象目录：

```yaml
storage:
  provider: local
  local:
    root: .gamedepot/blob-store
refs:
  root: depot/refs
```

`key_template` 支持：

```text
{sha256}
{sha256_2}   # 前2位
{sha256_4}   # 前4位
```

例如：

```yaml
key_template: objects/{sha256_2}/{sha256}
```

如果 Go 配置里只有：

```yaml
key_prefix: objects
```

工具会按 Go GameDepot 的 sharded layout 推导为：

```text
objects/sha256/aa/bb/<sha>.blob
```

### 可选覆盖文件

仍然保留 `.oss-manager.yaml` 作为临时覆盖文件，方便你只给这个 GUI 单独改 provider、key_template、ref_root 等。存在 `.gamedepot/config.yaml` 时，它只是覆盖补丁，不再作为主配置。

## 后台扫描与颜色说明

扫描分两段在后台线程执行，不会卡住 UI：

1. 状态栏显示 `正在扫描commit`：只扫描 Git 历史和 pointer ref，先把文件树/历史表列出来；
2. 状态栏显示 `正在扫描oss`：逐个检查 OSS 对象是否存在，并实时更新颜色。

左侧文件树和右侧历史表统一使用：

- 灰色：未知 / 待扫描 / 没有解析出 OSS 对象；
- 绿色：blob 存在于 OSS；
- 红色：blob 不存在于 OSS。

HEAD 已删除、但历史 commit 中仍存在的文件也会显示；它的颜色取最近一个历史 ref 对应的 blob 状态。

## 操作语义

### 文件右键

- 删除文件：删除工作区中的真实文件，以及对应的当前 `.gdref`。不会删除 OSS blob，也不会自动提交 Git。
- 仅保留当前版本：删除本文件历史中除当前版本外的旧 blob；如果某个旧 blob 仍被其它文件/历史引用，会默认跳过。
- 提取当前版本到：下载当前版本 blob 到指定路径。

### 历史右键

- 删除选中历史 OSS blob：删除该历史行对应的 OSS blob；Git commit 仍保留，但该版本之后不可恢复。
- 提取到...：下载该历史版本 blob 到指定路径。

## 安全策略

这个工具不会改写 Git 历史，只删除 OSS 上的 blob。删除前会弹出确认框，并提示引用次数。建议先在测试 bucket 或本地 provider 上验证。

## v0.1.1: commit 扫描优化

本版把左侧文件树的 commit 扫描改成单次 Git 日志流式扫描：

- 先用 `git rev-list --all --count` 估算总 commit 数；
- 再用 `git log --all --name-status` 一次性收集历史 ref 文件；
- 状态栏显示 `正在扫描commit：当前/总数`，进度条会走真实进度；
- 删除后仍存在于历史中的文件，不再对每个文件单独执行一次 `git log`，启动扫描会明显更快。

选择单个文件后，右侧历史提交读取也会显示 `正在扫描commit：读取历史 ref 当前/总数`。

## v0.1.2: 修复后台 worker 生命周期

修复 PyQt 中后台扫描 worker 可能被 Python 垃圾回收的问题。这个问题会导致界面一直停在 `正在扫描commit...`，但 Git 命令本身实际很快。新版会在 MainWindow 中持有 commit/OSS/history worker 引用，线程结束后再释放。

## v0.1.3: GameDepot OSS key 布局修正

修正 OSS key 推导规则，与 Go 版 GameDepot 一致：

```text
[prefix/]sha256/<前2位>/<第3-4位>/<完整sha>.blob
```

`.gdref` 中的 `path` 会被视为资产路径，不再被误认为 OSS object key。只有显式的 `key` / `objectKey` / `blobKey` 等字段才会作为 object key 使用。

## v0.1.4: 未知 Blob Meta 节点

左侧文件树新增：

```text
[Meta]
  未知 Blob (N)
```

未知 Blob 指：存在于当前项目配置的 OSS / prefix 空间中，但没有任何扫描到的 commit/ref 历史引用的 GameDepot blob-layout 对象。

行为：

- 扫描完普通文件 OSS 状态后，会继续后台扫描 `未知 Blob`；
- 状态栏会显示 `正在扫描commit：建立 blob 引用索引` 和 `正在扫描oss：未知 blob x/y`；
- 右键 `未知 Blob (N)` 可以一键删除全部未知 blob；
- 右键单个未知 blob 可以只删除该 blob；
- 删除前会弹确认框；删除后不修改 Git 历史。

为避免误删，未知 Blob 扫描只识别 GameDepot 标准布局对象：

```text
[prefix/]sha256/aa/bb/<64位sha>.blob
```

同一 prefix 下其它非 blob-layout 的对象会被忽略。
