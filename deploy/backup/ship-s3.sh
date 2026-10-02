#!/bin/sh
# The AWS CLI owns S3 transfer/retry parsing. This job never receives a Master Key.
set -eu
umask 077
if [ "$#" -ne 0 ]; then
    echo "usage: configure the backup environment, then run ship-s3.sh without arguments" >&2
    exit 64
fi

artifact=${BACKUP_ARTIFACT_DIRECTORY:-/work/artifact}
store=mysql
started=$(date +%s)
verify=
metrics=$(mktemp "${TMPDIR:-/tmp}/configra-backup-metrics.XXXXXX")
push() {
    curl --silent --fail --max-time 10 --request POST --data-binary "@$metrics" \
        "${PUSHGATEWAY_URL%/}/metrics/job/configra-backup/store/$store" >/dev/null 2>&1
}
cleanup() {
    result=$?
    if [ "$result" -ne 0 ]; then
        printf 'configra_backup_last_attempt_succeeded{store="mysql"} 0\nconfigra_backup_last_attempt_timestamp_seconds{store="mysql"} %s\n' "$(date +%s)" >"$metrics"
        if [ -n "${PUSHGATEWAY_URL:-}" ]; then push || true; fi
        echo "backup archive or verification failed" >&2
    fi
    rm -f "$metrics"
    if [ -n "$verify" ]; then rm -rf "$verify"; fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

case "${BACKUP_S3_PREFIX:-}" in s3://?*) ;; *) exit 64;; esac
case "${PUSHGATEWAY_URL:-}" in http://?*|https://?*) ;; *) exit 64;; esac
case "${POD_UID:-}" in ""|*[!a-zA-Z0-9-]*) exit 64;; esac
[ "${#POD_UID}" -le 64 ] || exit 64
for name in mysql.sql.gz manifest.sha256 recovery-point.timestamp metadata.txt; do
    [ -f "$artifact/$name" ] && [ ! -L "$artifact/$name" ] || exit 66
done
point=$(cat "$artifact/recovery-point.timestamp")
case "$point" in ""|*[!0-9]*) exit 65;; esac
[ "${#point}" -le 12 ] && [ "$point" -gt 0 ] && [ "$point" -le "$started" ] || exit 65
manifest=$(cat "$artifact/manifest.sha256")
expected=${manifest%% *}
case "$expected" in ""|*[!0-9a-f]*) exit 65;; esac
[ "${#expected}" -eq 64 ] && [ "$manifest" = "$expected  mysql.sql.gz" ] || exit 65
actual=$(sha256sum "$artifact/mysql.sql.gz"); actual=${actual%% *}
[ "$actual" = "$expected" ] || exit 65

# A new Pod UID creates a new recovery point; bucket retention must prevent
# overwriting/deleting older points. Never update a mutable "latest" object.
destination=${BACKUP_S3_PREFIX%/}/$POD_UID
verify=$(mktemp -d "${TMPDIR:-/tmp}/configra-backup-verify.XXXXXX")
export AWS_MAX_ATTEMPTS=3
export AWS_PAGER=
for name in mysql.sql.gz manifest.sha256 recovery-point.timestamp metadata.txt; do
    aws s3 cp "$artifact/$name" "$destination/$name" --checksum-algorithm SHA256 --only-show-errors >/dev/null 2>&1
    aws s3 cp "$destination/$name" "$verify/$name" --only-show-errors >/dev/null 2>&1
    local_hash=$(sha256sum "$artifact/$name"); local_hash=${local_hash%% *}
    remote_hash=$(sha256sum "$verify/$name"); remote_hash=${remote_hash%% *}
    [ "$local_hash" = "$remote_hash" ] || exit 74
done
finished=$(date +%s)
cat >"$metrics" <<EOF
configra_backup_last_attempt_succeeded{store="mysql"} 1
configra_backup_last_attempt_timestamp_seconds{store="mysql"} $finished
configra_backup_last_success_timestamp_seconds{store="mysql"} $finished
configra_backup_recovery_point_timestamp_seconds{store="mysql"} $point
configra_backup_archive_duration_seconds{store="mysql"} $((finished-started))
EOF
push || exit 75
echo "backup archived and verified"
