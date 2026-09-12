#!/bin/sh
set -eu

umask 077

if [ "$#" -ne 3 ]; then
    echo "usage: mysql-restore.sh DEFAULTS_FILE BACKUP_DIRECTORY NEW_DATABASE" >&2
    exit 64
fi

defaults_file=$1
backup_directory=$2
database=$3

case "$database" in
    ""|*[!A-Za-z0-9_]*)
        echo "invalid MySQL database name" >&2
        exit 64
        ;;
esac

dump_file=$backup_directory/mysql.sql.gz
manifest_file=$backup_directory/manifest.sha256
if [ ! -r "$defaults_file" ] || [ ! -f "$dump_file" ] || [ ! -f "$manifest_file" ]; then
    echo "MySQL defaults file or complete backup artifact is unavailable" >&2
    exit 66
fi

case "$(mysql --version)" in
    *" Ver 8.0.22 "*) ;;
    *)
        echo "mysql client must be version 8.0.22" >&2
        exit 69
        ;;
esac

server_version=$(mysql --defaults-extra-file="$defaults_file" --batch --skip-column-names -e "SELECT VERSION()")
if [ "$server_version" != "8.0.22" ]; then
    echo "MySQL server must be version 8.0.22" >&2
    exit 69
fi

manifest=$(cat "$manifest_file")
expected_checksum=${manifest%% *}
case "$expected_checksum" in
    ""|*[!0-9a-f]*)
        echo "invalid backup manifest" >&2
        exit 65
        ;;
esac
if [ "${#expected_checksum}" -ne 64 ] || [ "$manifest" != "$expected_checksum  mysql.sql.gz" ]; then
    echo "invalid backup manifest" >&2
    exit 65
fi
actual_checksum=$(sha256sum "$dump_file")
actual_checksum=${actual_checksum%% *}
if [ "$actual_checksum" != "$expected_checksum" ] || ! gzip -t "$dump_file"; then
    echo "backup checksum validation failed" >&2
    exit 65
fi

exists=$(mysql --defaults-extra-file="$defaults_file" --batch --skip-column-names \
    -e "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = '$database'")
if [ "$exists" != "0" ]; then
    echo "restore target database already exists" >&2
    exit 73
fi

sql_temporary=$(mktemp "${TMPDIR:-/tmp}/configra-restore.XXXXXX")
created=0
cleanup() {
    rm -f "$sql_temporary"
    if [ "$created" -eq 1 ]; then
        mysql --defaults-extra-file="$defaults_file" \
            -e "DROP DATABASE \`$database\`" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT HUP INT TERM

gzip -dc "$dump_file" >"$sql_temporary"
mysql --defaults-extra-file="$defaults_file" \
    -e "CREATE DATABASE \`$database\` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"
created=1
mysql --defaults-extra-file="$defaults_file" "$database" <"$sql_temporary"
created=0
