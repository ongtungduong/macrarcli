# Đánh giá tổng thể codebase macrarcli — Architecture / Performance / Security

Scope: full codebase (~1744 non-test LOC, 14 file Go), 3 reviewer song song. Branch `main` @ `d16298d`.

## Executive Summary

Không có Critical. Codebase nhỏ, gọn, kỷ luật tốt (streaming I/O, sanitize path traversal có test, staging+rollback). Vấn đề chính: 4 Important về security (perm bits, password argv, TOCTOU nhỏ, decompression-bomb cap mặc định tắt), 2 Important về performance (`--test`/`--list` không dùng `--jobs`, mutex global serialize batch commit), 2 Important về architecture (verbose flag không tới được `-l`, 3 report-loop trùng lặp không có abstraction chung).

Ưu tiên fix trước khi ship tiếp: **Security #1, #2, #4** (thấp effort, đóng lỗ thật) → sau đó Performance #1 (dễ, tận dụng `--jobs` đã có) → Architecture #1/#2 (cleanup, giảm rủi ro tái diễn bug tương tự đã fix trước đó — verbose không tới list y hệt pattern bug `password` không tới list đã fix ở main.go:187).

---

## 1. SECURITY (ưu tiên cao nhất)

### Important

1. **World-writable file từ archive-controlled perm bits** — `internal/rarutil/writer.go:172` (`emitFile`), `sanitize.go:45-51` (`safeMode`)
   `safeMode` chỉ strip symlink/device/setuid/setgid/sticky, giữ nguyên toàn bộ perm bits (tới 0777). `emitFile` chmod chính xác mode đó, không qua umask. Test hiện tại còn assert hành vi này (`safe_mode_test.go:21`).
   Exploit: RAR entry mode 0777 (tạo trên host unix) → file world-writable/executable sau extract. Trên `destDir` dùng chung (`/tmp`, checkout share) user khác local sửa được file trước khi nạn nhân đọc/chạy.
   Fix: `mode.Perm() &^ 0o022` trước khi chmod.

2. **`--password` lộ secret qua argv** — `main.go:89`
   `ps aux` / `/proc/<pid>/cmdline` đọc được password khi flag dùng trực tiếp (path TTY-prompt đã mask đúng qua `term.ReadPassword`). Không có env-var fallback cho automation.
   Fix: thêm `MACRARCLI_PASSWORD` env fallback, doc rõ `--password` không an toàn trên host share.

3. **TOCTOU symlink race ở fallback cross-filesystem rename** — `stage.go` (`renameOrCopy`, ~165-193)
   Khi `os.Rename` fail (khác filesystem), fallback `OpenFile` không `O_NOFOLLOW`, chạy sau `checkOverwrite`'s Lstat check → race window. Thực tế hiếm trigger vì staging nằm cùng `destDir` (`makeStagingDir`) nên rename gần như luôn same-fs. Severity thực tế thấp hơn code path gợi ý.
   Fix: `O_NOFOLLOW` (unix) hoặc re-Lstat ngay trước open.

4. **Decompression-bomb cap mặc định = unlimited** — `cli_args.go:92-93` (`--max-size`/`--max-entries` default 0)
   Cơ chế cap implement đúng, nhưng opt-in. Archive nhỏ crafted có thể fill disk/inode mặc định.
   Fix: default non-zero cap, hoặc bắt buộc flag `--unsafe-no-limits` để tắt.

### Minor
5. Fd leak ở encryption-probe `rardecode.OpenReader` — `main.go:151` (không `Close()` khi `openErr == nil`). Negligible cho short-lived CLI.
6. Password là Go string thường trú heap tới GC (không zero được) — giới hạn ngôn ngữ, không phải bug code, không đề xuất fix (effort/benefit không đáng).

### Điểm tốt đã verify
- Zip-Slip/symlink defenses rigor cao, có test exploit thật (`TestCommitStaged_RefusesSymlinkedDirectoryInPath`, `TestCommitStaged_OverwriteSymlink`), không chỉ happy-path.
- Stage-then-commit-with-rollback: fail giữa chừng không để lại state nửa vời ở `destDir`.
- `install.sh`: checksum verify fatal-by-default, không có combo bypass ẩn; cosign additive.
- Không panic nào từ archive-controlled input tìm thấy trong code tự viết — mọi lỗi từ `rr.Next()`/`Read()` đều wrap & return.
- Prior Critical bug (list mode dùng raw `password` thay vì `resolvedPassword`) đã fix, verify tại `main.go:187`.

---

## 2. PERFORMANCE

### Important

1. **`--test`/`--list` bỏ qua `--jobs`, chạy sequential** — `main.go:314-320` (`runTest`), `list_output.go:24-27` (`runList`)
   Extract đi qua `rarutil.RunBatch` (concurrent), test/list loop plain. `--jobs 4 -t` trên 20 archive = 0 speedup.
2. **`commitMu` là 1 mutex global**, không per-destDir — `stage.go:19`
   Serialize toàn bộ walk+rename commit phase across mọi batch job kể cả khi đích khác nhau hoàn toàn, làm giảm hiệu quả `--jobs` cho archive nhiều file nhỏ.

### Minor
3. `writer.go:155` — `os.MkdirAll` gọi per-file thay vì per-dir → 1 stat syscall thừa/file khi parent dir đã tồn tại.

### Điểm tốt đã verify
- Không `ReadAll`/`ReadFile` archive content ở đâu (grep-verify) — streaming `io.CopyBuffer` toàn bộ, buffer pool 512KB.
- Không benchmark test nào tồn tại (`func Benchmark` = 0) — gap, không phải regression.

Unresolved: sequential test/list là intentional (đảm bảo stderr order deterministic) hay oversight? Cần xác nhận trước khi fix #1.

---

## 3. ARCHITECTURE

### Important

1. **`runList` không nhận được verbose/options đầy đủ; `--verbose` no-op im lặng dưới `-l`** — `list_output.go:21`
   Signature `runList(inputs, password, maxEntries, jsonOut)` không có `OnVerbose`, main.go:187 cũng không truyền verbose bool xuống. Same class bug với password-list bug đã fix trước — pattern lặp lại vì list-mode plumbing tách biệt khỏi `Options` struct dùng chung cho extract/test.
2. **3 report-loop trùng lặp** (`main.go:282-309` report, `main.go:314-342` runTest, `list_output.go` runList/printList/listExitCodes) không share abstraction, trong khi JSON side đã có `writeJSONSummary` chung. Đây chính là nguyên nhân gốc cho phép bug #1 xảy ra.

### Minor
3. `RunBatch`'s `onStart` callback dead code (luôn gọi `nil`), doc gọi nhầm tên `onProgress` (`codebase-summary.md:135`) — semantics khác (fire 1 lần lúc start, không phải progress tick).
4. File-size/doc drift: `list_output.go` 203 dòng (doc claim ~50) trộn 3 concern (dispatch, table render, JSON); `writer.go` 201 dòng, vượt hard cap 200 dòng của chính project.
5. Doc drift khác: `system-architecture.md` data-flow diagram thiếu `reportListJSON`; LOC table stale.

### Điểm tốt đã verify
- `internal/rarutil` zero import `main`/CLI concern — package boundary sạch.
- `dedupVariant` share đúng 1 chỗ giữa overwrite.go và writer.go, không duplicate logic.
- Error classification centralized, sentinel-based (`classifyErr`/`aggregateExit`), reuse nhất quán.
- Thiếu abstraction cho multi-format archive là chủ đích (pure-Go, single-format, no-shell-out thesis), không phải defect.

---

## Khuyến nghị hành động (ưu tiên)

1. Cap perm bits khi extract (Security #1) — effort thấp, đóng lỗ thật.
2. Env-var password fallback + doc cảnh báo `--password` (Security #2).
3. Default non-zero decompression-bomb cap (Security #4).
4. Route `--jobs` qua test/list (Performance #1) — dùng lại `RunBatch` đã có sẵn.
5. Gộp `runList` vào `Options`-based signature giống extract/test, fix verbose no-op (Architecture #1) — cùng lúc giải quyết root cause của #2 (report-loop duplication) nếu refactor thành 1 reporter chung.
6. Backlog (không urgent): O_NOFOLLOW fallback rename (Security #3), per-destDir commit lock (Performance #2), split `list_output.go`/`writer.go` theo 200-line cap, xóa hoặc wire `onStart`, đồng bộ docs.

## Unresolved Questions
- `rardecode/v2` có bao giờ trả perm bits >0755 cho archive tạo trên non-unix host không? Ảnh hưởng mức độ exploitable thực tế của Security #1.
- `destDir` có thực tế nào là mount point/bind-mount trong deployment thật không? Ảnh hưởng ưu tiên fix Security #3.
- Sequential test/list (Performance #1) — intentional cho deterministic output hay oversight?
- `RunBatch.onStart` — giữ cho tính năng tương lai hay bỏ theo YAGNI?
- `--verbose` dưới `-l`/`-t` — scope quyết định có chủ đích hay gap? Cần xác nhận trước khi coi Architecture #1 là bug cần fix vs. cần doc rõ.
