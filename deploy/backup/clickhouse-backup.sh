#!/bin/sh
set -eu

umask 077

if [ "$#" -ne 4 ]; then
    echo "usage: clickhouse-backup.sh CLIENT_CONFIG DATABASE DISK ARCHIVE.zip" >&2
    exit 64
fi

client_config=$1
database=$2
disk=$3
archive=$4

for identifier in "$database" "$disk"; do
    case "$identifier" in
        ""|[0-9]*|*[!A-Za-z0-9_]*)
            echo "invalid ClickHouse identifier" >&2
            exit 64
            ;;
    esac
done
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

clickhouse-client --config-file="$client_config" \
    --query "BACKUP DATABASE $database TO Disk('$disk', '$archive')"
