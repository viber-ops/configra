#!/bin/sh
set -eu

umask 077

if [ "$#" -ne 3 ]; then
    echo "usage: mysql-backup.sh DEFAULTS_FILE DATABASE OUTPUT_DIRECTORY" >&2
    exit 64
fi

defaults_file=$1
database=$2
output_directory=$3

case "$database" in
    ""|*[!A-Za-z0-9_]*)
        echo "invalid MySQL database name" >&2
        exit 64
        ;;
esac

if [ ! -r "$defaults_file" ] || [ ! -d "$output_directory" ]; then
    echo "MySQL defaults file or output directory is unavailable" >&2
    exit 66
fi

dump_file=$output_directory/mysql.sql.gz
manifest_file=$output_directory/manifest.sha256
if [ -e "$dump_file" ] || [ -L "$dump_file" ] || [ -e "$manifest_file" ] || [ -L "$manifest_file" ]; then
    echo "backup output already exists" >&2
    exit 73
fi

case "$(mysqldump --version)" in
    *" Ver 8.0.22 "*) ;;
    *)
        echo "mysqldump must be version 8.0.22" >&2
        exit 69
        ;;
esac

server_version=$(mysql --defaults-extra-file="$defaults_file" --batch --skip-column-names -e "SELECT VERSION()")
if [ "$server_version" != "8.0.22" ]; then
    echo "MySQL server must be version 8.0.22" >&2
    exit 69
fi

sql_temporary=$(mktemp "$output_directory/.mysql.sql.XXXXXX")
gzip_temporary=$(mktemp "$output_directory/.mysql.sql.gz.XXXXXX")
manifest_temporary=$(mktemp "$output_directory/.manifest.sha256.XXXXXX")
cleanup() {
    rm -f "$sql_temporary" "$gzip_temporary" "$manifest_temporary"
}
trap cleanup EXIT HUP INT TERM

mysqldump --defaults-extra-file="$defaults_file" \
    --single-transaction \
    --quick \
    --hex-blob \
    --set-gtid-purged=OFF \
    --no-tablespaces \
    --column-statistics=0 \
    --skip-comments \
    "$database" >"$sql_temporary"

if [ ! -s "$sql_temporary" ]; then
    echo "mysqldump produced no data" >&2
    exit 74
fi

gzip -c "$sql_temporary" >"$gzip_temporary"
checksum=$(sha256sum "$gzip_temporary")
checksum=${checksum%% *}
printf '%s  mysql.sql.gz\n' "$checksum" >"$manifest_temporary"

mv "$gzip_temporary" "$dump_file"
mv "$manifest_temporary" "$manifest_file"
