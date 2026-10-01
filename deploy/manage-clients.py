#!/usr/bin/env python3
import argparse
import json
import os
import grp
import secrets
import sys
import tempfile
import uuid
from datetime import datetime, timezone

DEFAULT_PATH = "/etc/chameleon/clients.json"

def load(path):
    if not os.path.exists(path):
        return {"schema": 1, "clients": []}
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    if data.get("schema") != 1 or not isinstance(data.get("clients"), list):
        raise SystemExit("unsupported clients.json schema")
    return data

def save(path, data):
    directory = os.path.dirname(path) or "."
    os.makedirs(directory, exist_ok=True)
    fd, tmp = tempfile.mkstemp(prefix=".clients-", dir=directory, text=True)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            json.dump(data, fh, indent=2, ensure_ascii=False)
            fh.write("\n")
        os.chmod(tmp, 0o640)
        os.replace(tmp, path)
        try:
            os.chown(path, 0, grp.getgrnam("chameleon").gr_gid)
        except (KeyError, PermissionError):
            pass
    finally:
        if os.path.exists(tmp):
            os.unlink(tmp)

def find(data, client_id):
    for client in data["clients"]:
        if client.get("id") == client_id:
            return client
    raise SystemExit(f"client not found: {client_id}")

parser = argparse.ArgumentParser(description="Manage Chameleon Auth v2 clients")
parser.add_argument("--file", default=DEFAULT_PATH)
sub = parser.add_subparsers(dest="command", required=True)

sub.add_parser("list")
add = sub.add_parser("add")
add.add_argument("name", nargs="?", default="Android client")
add.add_argument("--expires", default="")

for command in ("enable", "disable", "delete"):
    p = sub.add_parser(command)
    p.add_argument("client_id")

args = parser.parse_args()
data = load(args.file)

if args.command == "list":
    for client in data["clients"]:
        print("\t".join([
            str(client.get("id", "")),
            str(client.get("name", "")),
            "enabled" if client.get("enabled") else "disabled",
            str(client.get("expires_at", "") or "-"),
        ]))
    raise SystemExit(0)

if args.command == "add":
    if args.expires:
        try:
            datetime.fromisoformat(args.expires.replace("Z", "+00:00"))
        except ValueError as exc:
            raise SystemExit(f"invalid --expires timestamp: {exc}")
    client = {
        "id": str(uuid.uuid4()),
        "name": args.name,
        "secret": secrets.token_hex(32),
        "enabled": True,
    }
    if args.expires:
        client["expires_at"] = args.expires
    data["clients"].append(client)
    save(args.file, data)
    print(f"CHAMELEON_CLIENT_ID={client['id']}")
    print(f"CHAMELEON_CLIENT_SECRET={client['secret']}")
    raise SystemExit(0)

client = find(data, args.client_id)
if args.command == "enable":
    client["enabled"] = True
elif args.command == "disable":
    client["enabled"] = False
elif args.command == "delete":
    data["clients"] = [c for c in data["clients"] if c.get("id") != args.client_id]

save(args.file, data)
print(f"{args.command}: {args.client_id}")
