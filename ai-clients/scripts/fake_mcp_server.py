#!/usr/bin/env python3
"""Servidor MCP falso para validar los scripts de ai-clients/ mientras el backend no está en la rama.

Responde POST /mcp con JSON-RPC: initialize, notifications/initialized (202), tools/list y ping.
Exige la cabecera "Authorization: Bearer <EBK_MCP_TOKEN>"; la variable debe estar definida en el proceso del servidor.
No guarda datos ni toca nada del sistema.

Uso (desde la raíz del repo):
    EBK_MCP_TOKEN=<token de prueba> python -I ai-clients/scripts/fake_mcp_server.py [--port 8080] [--omit NOMBRE]

--omit quita una herramienta de tools/list. Sirve para comprobar que check-mcp sale con código distinto de 0.
"""
import argparse
import json
import os
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PROTOCOL_VERSION = "2025-06-18"

# Las 10 herramientas que exige check-mcp, más las que ya trae upstream y no usan los comandos.
TOOLS = [
    "list_inbox_items",
    "get_inbox_item",
    "resolve_inbox_item",
    "discard_inbox_item",
    "add_transactions_batch",
    "update_transaction",
    "get_spending_summary",
    "query_transactions",
    "query_all_accounts",
    "query_all_transaction_categories",
    "add_transaction",
    "query_all_accounts_balance",
    "query_all_transaction_tags",
    "query_latest_exchange_rates",
]

OMITTED = set()


class Handler(BaseHTTPRequestHandler):
    server_version = "fake-ebk-mcp/0.1"

    def log_message(self, fmt, *args):
        # Una línea por petición; nunca se registran cabeceras ni cuerpos.
        sys.stderr.write("[fake-mcp] %s\n" % (fmt % args))

    def _send(self, status, body=None):
        data = b"" if body is None else json.dumps(body).encode("utf-8")
        self.send_response(status)
        if body is not None:
            self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        if data:
            self.wfile.write(data)

    def _authorized(self):
        expected = os.environ.get("EBK_MCP_TOKEN", "")
        header = self.headers.get("Authorization", "")
        return bool(expected) and header[:7].lower() == "bearer " and header[7:] == expected

    def do_GET(self):
        self._send(405, {"success": False, "error": {"message": "method not allowed"}})

    def do_POST(self):
        if self.path != "/mcp":
            self._send(404, {"success": False, "error": {"message": "not found"}})
            return
        if not self._authorized():
            self._send(401, {"success": False, "error": {"message": "token MCP ausente o incorrecto"}})
            return

        length = int(self.headers.get("Content-Length", "0") or 0)
        try:
            request = json.loads(self.rfile.read(length).decode("utf-8"))
        except (ValueError, UnicodeDecodeError):
            self._send(400, {"jsonrpc": "2.0", "id": None, "error": {"code": -32700, "message": "Parse error"}})
            return
        if not isinstance(request, dict):
            self._send(400, {"jsonrpc": "2.0", "id": None, "error": {"code": -32600, "message": "Invalid Request"}})
            return

        method = request.get("method")
        if method == "notifications/initialized":
            self._send(202)
            return

        if method == "initialize":
            result = {
                "protocolVersion": PROTOCOL_VERSION,
                "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "ezbookkeeping-fake", "version": "0.0.0-test"},
            }
        elif method == "tools/list":
            result = {
                "tools": [
                    {
                        "name": name,
                        "description": "Herramienta falsa para validar los scripts de ai-clients",
                        "inputSchema": {"type": "object", "properties": {}},
                    }
                    for name in TOOLS
                    if name not in OMITTED
                ]
            }
        elif method == "ping":
            result = {}
        else:
            self._send(200, {"jsonrpc": "2.0", "id": request.get("id"), "error": {"code": -32601, "message": "Method not found"}})
            return

        self._send(200, {"jsonrpc": "2.0", "id": request.get("id"), "result": result})


def main():
    parser = argparse.ArgumentParser(description="Servidor MCP falso para validar ai-clients/scripts.")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8080)
    parser.add_argument("--omit", action="append", default=[], metavar="NOMBRE",
                        help="quita una herramienta de tools/list (repetible)")
    args = parser.parse_args()

    if not os.environ.get("EBK_MCP_TOKEN"):
        sys.stderr.write("[fake-mcp] define EBK_MCP_TOKEN antes de arrancar el servidor\n")
        return 2

    unknown = [name for name in args.omit if name not in TOOLS]
    if unknown:
        sys.stderr.write("[fake-mcp] herramientas desconocidas en --omit: %s\n" % ", ".join(unknown))
        return 2
    OMITTED.update(args.omit)

    server = ThreadingHTTPServer((args.host, args.port), Handler)
    sys.stdout.write("[fake-mcp] escuchando en http://%s:%d/mcp (omitidas: %s)\n"
                     % (args.host, args.port, ", ".join(sorted(OMITTED)) or "ninguna"))
    sys.stdout.flush()
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
