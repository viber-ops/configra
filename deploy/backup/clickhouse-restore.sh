#!/bin/sh
set -eu

umask 077

if [ "$#" -ne 5 ]; then
    echo "usage: clickhouse-restore.sh CLIENT_CONFIG SOURCE_DATABASE DISK ARCHIVE.zip NEW_DATABASE" >&2
    exit 64
fi

client_config=$1
source_database=$2
disk=$3
archive=$4
target_database=$5

for identifier in "$source_database" "$disk" "$target_database"; do
    case "$identifier" in
        ""|[0-9]*|*[!A-Za-z0-9_]*)
            echo "invalid ClickHouse identifier" >&2
            exit 64
            ;;
    esac
done
if [ "$source_database" = "$target_database" ]; then
    echo "ClickHouse restore target must be a new database" >&2
    exit 64
fi
case "$archive" in
    ""|*[!A-Za-z0-9._-]*|*..*|.*|*.zip.zip)
        echo "invalid ClickHouse backup archive" >&2
        exit 64
        ;;
    *.zip) ;;
    *)
        echo "ClickHouse backup archive must end in .zip" >&2
        exit 64
        ;;
esac
if [ "${#archive}" -gt 128 ] || [ ! -r "$client_config" ]; then
    echo "ClickHouse client config or archive name is invalid" >&2
    exit 66
fi

case "$(clickhouse-client --version)" in
    *"version 26.7.3.19"*) ;;
    *)
        echo "clickhouse-client must be version 26.7.3.19" >&2
        exit 69
        ;;
esac
server_version=$(clickhouse-client --config-file="$client_config" --query "SELECT version()")
if [ "$server_version" != "26.7.3.19" ]; then
    echo "ClickHouse server must be version 26.7.3.19" >&2
    exit 69
fi

owned_target=0
cleanup() {
    if [ "$owned_target" -eq 1 ]; then
        clickhouse-client --config-file="$client_config" \
            --query "DROP DATABASE IF EXISTS $target_database SYNC" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT HUP INT TERM

clickhouse-client --config-file="$client_config" --query "CREATE DATABASE $target_database"
owned_target=1
clickhouse-client --config-file="$client_config" \
    --query "RESTORE DATABASE $source_database AS $target_database FROM Disk('$disk', '$archive')"
owned_target=0
