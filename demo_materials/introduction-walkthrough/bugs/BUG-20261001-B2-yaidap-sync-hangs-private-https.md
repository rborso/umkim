# BUG: yaidap-sync hangs on private HTTPS git remote

## Context
Phase B2: `umkim site provision set` with YAIDAP repo `https://github.com/rborso/yaidap.git` triggered a sync job that hung on non-interactive `git fetch` (no credentials / deploy key for HTTPS).

## Impact
Site provision / YAIDAP bind cannot complete until the job is deleted; operator must re-bind without `--yaidap-repo` and use an existing jumphost checkout, or configure SSH deploy key / credential helper.

## Workaround
Delete hung job; re-bind provision to existing `/home/linux/yaidap` on jumphost without repo URL sync.
