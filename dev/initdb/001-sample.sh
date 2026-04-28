#!/usr/bin/env sh
set -eu

sql_escape() {
  printf '%s' "$1" | sed "s/'/''/g"
}

readonly_user="$(sql_escape "${PMA_GATEWAY_BOOTSTRAP_DEV_READONLY_DB_USER:-readonly_user}")"
readonly_password="$(sql_escape "${PMA_GATEWAY_BOOTSTRAP_DEV_READONLY_DB_PASSWORD:-}")"
admin_user="$(sql_escape "${PMA_GATEWAY_BOOTSTRAP_DEV_ADMIN_DB_USER:-admin_user}")"
admin_password="$(sql_escape "${PMA_GATEWAY_BOOTSTRAP_DEV_ADMIN_DB_PASSWORD:-}")"

mariadb -uroot -p"${MARIADB_ROOT_PASSWORD}" <<SQL
CREATE DATABASE IF NOT EXISTS sampledb;

CREATE TABLE IF NOT EXISTS sampledb.widgets (
  id INT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO sampledb.widgets(name)
VALUES ('alpha'), ('beta'), ('gamma')
ON DUPLICATE KEY UPDATE name = VALUES(name);

CREATE USER IF NOT EXISTS '${readonly_user}'@'%' IDENTIFIED BY '${readonly_password}';
CREATE USER IF NOT EXISTS '${admin_user}'@'%' IDENTIFIED BY '${admin_password}';

GRANT SELECT ON sampledb.* TO '${readonly_user}'@'%';
GRANT ALL PRIVILEGES ON sampledb.* TO '${admin_user}'@'%';
FLUSH PRIVILEGES;
SQL
