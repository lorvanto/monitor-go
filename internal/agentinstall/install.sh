#!/bin/sh
# Monitor Go Linux Agent installer. The enrollment token is read only from stdin.
set -eu
umask 077

die() { printf 'Monitor Go Agent: %s\n' "$1" >&2; exit 1; }
server=
replace=false
while [ "$#" -gt 0 ]; do
    case "$1" in
        --server) [ "$#" -ge 2 ] || die '--server requires an origin.'; server=$2; shift 2 ;;
        --replace) replace=true; shift ;;
        *) die 'Usage: sh install.sh --server <HTTPS origin> [--replace] (token on stdin).' ;;
    esac
done
[ -n "$server" ] || die '--server is required.'
server=${server%/}
for tool in curl sha256sum uname id systemctl getent useradd install mktemp flock grep cut cat chmod chown cp mv mkdir rm; do
    command -v "$tool" >/dev/null 2>&1 || die "Required command is missing: $tool"
done
[ "$(uname -s)" = Linux ] || die 'Only Linux with systemd is supported.'
[ "$(id -u)" = 0 ] || die 'Run this installer as root.'
system_state=$(systemctl is-system-running 2>/dev/null) || :
case "$system_state" in
    running|degraded|starting) ;;
    *) die "Unsupported systemd state: ${system_state:-unknown}; running, degraded or starting is required." ;;
esac
systemctl show --property=SystemState --value >/dev/null 2>&1 || die "Cannot contact the systemd system manager (state: $system_state)."
case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) die 'Only Linux amd64 and arm64 are supported.' ;;
esac
printf '%s\n' "$server" | LC_ALL=C grep -Eq '^https?://([A-Za-z0-9._-]+|\[[0-9A-Fa-f:.]+\])(:[0-9]+)?$' || die 'The server must be an origin without a path, query, fragment or credentials.'
case "$server" in
    https://*) protocols='=https' ;;
    http://*)
        authority=${server#http://}
        case "$authority" in
            \[*) host=${authority%%\]*}; host=${host#\[}; printf '%s\n' "$host" | grep -Eq '^[0:]+:0*1$' || die 'HTTP is allowed only for numeric loopback addresses.' ;;
            *)
                host=${authority%%:*}
                case "$host" in 127.*) ;; *) die 'HTTP is allowed only for numeric loopback addresses.' ;; esac
                saved_ifs=$IFS; IFS=.; set -- $host; IFS=$saved_ifs
                [ "$#" -eq 4 ] || die 'Invalid loopback IPv4 address.'
                for octet do
                    case "$octet" in ''|*[!0-9]*|0[0-9]*) die 'Invalid loopback IPv4 address.' ;; esac
                    [ "$octet" -le 255 ] || die 'Invalid loopback IPv4 address.'
                done
                ;;
        esac
        protocols='=http'
        ;;
    *) die 'Remote installations require HTTPS.' ;;
esac
enrollment_token=
IFS= read -r enrollment_token || [ -n "$enrollment_token" ] || die 'Read one enrollment token line from stdin.'
[ "${#enrollment_token}" -eq 64 ] || die 'Invalid enrollment token format.'
case "$enrollment_token" in *[!0-9A-Fa-f]*) die 'Invalid enrollment token format.' ;; esac
identity=$(printf '%s\n%s\n' "$server" "$enrollment_token" | sha256sum | cut -d ' ' -f 1)

temporary=$(mktemp -d "${TMPDIR:-/tmp}/monitor-go-agent.XXXXXXXX")
trap 'rm -rf "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
printf '%s\n' "$enrollment_token" > "$temporary/enrollment-token"
unset enrollment_token
download() {
    status=$(curl --disable --silent --show-error --fail --proto "$protocols" --tlsv1.2 --connect-timeout 10 --max-time 180 --output "$2" --write-out '%{http_code}' "$1") || die 'Download failed; the existing Agent has not been changed.'
    [ "$status" = 200 ] || die 'Download returned an unexpected status; redirects are not accepted.'
}
download "$server/api/v1/agent/download/linux/$arch/sha256" "$temporary/checksum"
expected=$(cat "$temporary/checksum")
[ "${#expected}" -eq 64 ] || die 'The server returned an invalid SHA-256 checksum.'
case "$expected" in *[!0-9a-f]*) die 'The server returned an invalid SHA-256 checksum.' ;; esac
download "$server/api/v1/agent/download/linux/$arch" "$temporary/agent"
actual=$(sha256sum "$temporary/agent" | cut -d ' ' -f 1)
[ "$actual" = "$expected" ] || die 'The downloaded Agent failed SHA-256 verification; nothing was installed.'
chmod 0755 "$temporary/agent"

service_user=monitor-go-agent
state_dir=/var/lib/monitor-go-agent
binary=/usr/local/bin/monitor-go-agent
unit=/etc/systemd/system/monitor-go-agent.service
config=$state_dir/config.json
marker=$state_dir/install-identity
lock=/run/monitor-go-agent-install.lock
for path in "$state_dir" "$state_dir/backups" "$config" "$config.lock" "$marker" "$binary" "$binary.new" "$unit" "$lock"; do
    [ ! -L "$path" ] || die 'Refusing to replace an installation path that is a symbolic link.'
done
exec 9> "$lock"
flock -n 9 || die 'Another Agent installation is running.'
if [ -e "$config" ]; then
    if [ -f "$marker" ] && [ "$(cat "$marker")" = "$identity" ]; then
        replace=false
    elif [ "$replace" != true ]; then
        die 'This machine has a different Agent installation. Use a new enrollment token and --replace to replace it explicitly.'
    fi
elif [ -e "$marker" ] && [ "$(cat "$marker")" != "$identity" ] && [ "$replace" != true ]; then
    die 'A different pending installation exists; use --replace with a new enrollment token.'
fi
if getent passwd "$service_user" >/dev/null; then
    [ "$(id -u "$service_user")" != 0 ] || die 'The Agent service account must not be root.'
    [ "$(id -gn "$service_user")" = "$service_user" ] || die 'The Agent service account needs its dedicated group.'
else
    useradd --system --user-group --no-create-home --home-dir "$state_dir" --shell /usr/sbin/nologin "$service_user"
fi
install -d -m 0700 -o "$service_user" -g "$service_user" "$state_dir"
for directory in "${binary%/*}" "${unit%/*}"; do
    [ -d "$directory" ] || install -d -m 0755 "$directory"
done
if systemctl cat monitor-go-agent.service >/dev/null 2>&1; then
    systemctl stop monitor-go-agent.service
fi
if [ "$replace" = true ] && { [ -e "$config" ] || [ -e "$marker" ]; }; then
    install -d -m 0700 "$state_dir/backups"
    backup=$(mktemp -d "$state_dir/backups/credentials.XXXXXXXX")
    for file in "$config" "$marker"; do
        if [ -f "$file" ]; then
            cp "$file" "$backup/"
            chmod 0600 "$backup/${file##*/}"
        fi
    done
    rm -f "$config" "$marker"
    printf 'Previous credentials preserved in %s.\n' "$backup"
fi
install -m 0755 "$temporary/agent" "$binary.new"
mv -f "$binary.new" "$binary"
printf '%s\n' "$identity" > "$temporary/identity"
install -m 0600 "$temporary/identity" "$marker"
bootstrap_status=0
"$binary" -once -server "$server" -enrollment-file "$temporary/enrollment-token" -config "$config" || bootstrap_status=$?
for file in "$config" "$config.lock"; do
    if [ -f "$file" ]; then
        chmod 0600 "$file"
        chown "$service_user:$service_user" "$file"
    fi
done
[ "$bootstrap_status" -eq 0 ] || die 'Registration or the first report failed. Credentials were preserved; repeat the same command to retry.'
cat > "$temporary/service" <<'MONITOR_GO_SERVICE'
[Unit]
Description=Monitor Go Host Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=monitor-go-agent
Group=monitor-go-agent
StateDirectory=monitor-go-agent
StateDirectoryMode=0700
ExecStart=/usr/local/bin/monitor-go-agent -config /var/lib/monitor-go-agent/config.json
Restart=on-failure
RestartSec=15
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=true
UMask=0077

[Install]
WantedBy=multi-user.target
MONITOR_GO_SERVICE
install -m 0644 "$temporary/service" "$unit"
systemctl daemon-reload
systemctl enable --now monitor-go-agent.service
systemctl is-active --quiet monitor-go-agent.service || die 'The first report succeeded but the Agent service is not active; credentials were preserved.'
printf '%s\n' 'Monitor Go Agent installed; registration and the first report succeeded.'
