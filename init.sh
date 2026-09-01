#!/usr/bin/env bash
#
# bash init.sh — ¿está esta máquina lista para correr el bot?
#
# Cuatro chequeos, todos por fallas ya vistas. cmd/server/main.go se traga el
# error de arranque y sale 1 sin decir nada, así que un arranque roto y un
# crash se ven igual. Esto los separa ANTES de arrancar.
#
# No chequea herramientas instaladas: check.sh ya avisa si falta errcheck.

set -u
cd "$(dirname "$0")" || exit 1

status=0
fail() { echo "  FALTA: $*"; status=1; }
ok()   { echo "  ok: $*"; }

echo "== 1. Postgres =="
if docker compose ps --status running 2>/dev/null | grep -q postgres; then
	ok "corriendo"
else
	fail "Postgres no está arriba. Corré: docker compose up -d"
fi

echo "== 2. .env =="
if [ ! -f .env ]; then
	fail ".env no existe. Copialo de .env.example y completalo."
elif grep -q $'\r' .env; then
	fail ".env tiene finales de línea Windows (CRLF). Cada valor arrastra un \\r y Telegram rechaza el token. Arreglalo con: tr -d '\\r' < .env > .env.tmp && mv .env.tmp .env"
else
	ok "presente, sin CRLF"
fi

echo "== 3. secretos en .env =="
if [ -f .env ]; then
	missing=0
	for var in ENV TELEGRAM_TOKEN GROQ_APIKEY; do
		value=$(grep "^${var}=" .env | head -1 | cut -d= -f2- | tr -d '\r')
		if [ -z "$value" ]; then
			fail "$var está vacía o ausente en .env"
			missing=1
		fi
	done
	[ "$missing" -eq 0 ] && ok "las tres están"
fi

echo "== 4. migración de admin =="
admin_migration='migrations/20260618230837_create_admin_user.sql'
if [ ! -f "$admin_migration" ]; then
	fail "no encuentro $admin_migration"
elif grep -q "'TELEGRAM_ID'" "$admin_migration"; then
	fail "$admin_migration todavía tiene el literal 'TELEGRAM_ID'. Reemplazalo por tu id numérico de Telegram antes del primer arranque."
else
	ok "editada"
fi

echo ""
if [ "$status" -eq 0 ]; then
	echo "init: OK — arrancá con:  cd cmd/server && set -a && . ../../.env && set +a && go run ."
else
	echo "init: FALTAN COSAS (arriba). El túnel y el id de Telegram no los puedo chequear yo: docs/dev-setup.md."
fi
exit "$status"
