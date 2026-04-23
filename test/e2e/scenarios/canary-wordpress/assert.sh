#!/usr/bin/env bash
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../lib.sh
source "$SCRIPT_DIR/../../lib.sh"

: "${PROJECT:?}"
: "${GOCKER:?}"

fail_count=0

# 1. MariaDB accepts connections. The real readiness probe is 'mariadb-admin ping'.
if retry_exec 90 "$PROJECT" db -- mariadb-admin ping -h localhost -uroot -proot; then
    log_pass "mariadb responds to ping"
else
    log_fail "mariadb never became reachable within 90s"
    fail_count=$((fail_count + 1))
fi

# 2. HTTP response from wordpress. WordPress returns 200 (installer page) or
#    302 (redirect to /wp-admin/install.php). Either proves apache is serving
#    and php/mysql wiring is OK. The wordpress image ships curl, not wget.
#    A successful HTTP response subsumes "apache started" — simpler + more
#    meaningful than scraping logs for 'AH00094'.
if RETRY_MATCH='HTTP/1.[01] (200|302)' retry_exec_capture 180 "$PROJECT" wordpress -- \
       curl -sSI http://localhost/ >/dev/null; then
    log_pass "wordpress responds with 200/302 on HTTP"
else
    log_fail "wordpress did not return a 200/302 response within 180s"
    fail_count=$((fail_count + 1))
fi

# 3. Sanity: wordpress→db wiring. A 302 to /wp-admin/install.php is only
#    produced once wp-config.php was written with working DB credentials, so
#    the redirect target itself is evidence the db is reachable from wordpress.
if RETRY_MATCH='Location: .*/wp-admin/install.php' retry_exec_capture 60 "$PROJECT" wordpress -- \
       curl -sSI http://localhost/ >/dev/null; then
    log_pass "wordpress redirects to installer (db connectivity OK)"
else
    log_fail "wordpress did not redirect to installer (db wiring broken?)"
    fail_count=$((fail_count + 1))
fi

exit "$fail_count"
