#!/usr/bin/env python3
"""Проверка доступности сервера из РФ и из-за рубежа через Globalping.

Отвечает на вопрос «режет ли ТСПУ (РКН) TCP до этого IP».
Только стандартная библиотека, работает на Windows/Linux/macOS.

Примеры:
  python check.py proxy.example.com:2443
  python check.py 203.0.113.10:2443 198.51.100.7:443 --ru 10
  python check.py --file servers.txt          # строки "имя host:port"
Токен (поднимает квоту с 250 до 500 проверок/час): env GLOBALPING_TOKEN.
"""
import argparse
import json
import os
import socket
import sys
import time
import urllib.error
import urllib.request

API = "https://api.globalping.io/v1/measurements"
FOREIGN = ["DE", "NL", "FI"]  # контроль: по одному зонду из каждой страны
state = {"remaining": None}


class QuotaError(Exception):
    pass

try:
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass


def api(method, url, body=None):
    headers = {"Content-Type": "application/json", "Accept-Encoding": "identity"}
    tok = os.environ.get("GLOBALPING_TOKEN")
    if tok:
        headers["Authorization"] = "Bearer " + tok
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            rem = r.headers.get("X-RateLimit-Remaining")
            if rem is not None:
                state["remaining"] = rem
            return json.load(r)
    except urllib.error.HTTPError as e:
        msg = e.read().decode(errors="replace")[:300]
        if e.code == 429:
            sys.exit("Globalping: квота исчерпана (429). Подожди до часа или задай GLOBALPING_TOKEN.\n" + msg)
        raise RuntimeError(f"Globalping HTTP {e.code}: {msg}")


def ru_locations(n):
    """Половина зондов — домашние провайдеры (их ТСПУ и видят клиенты), половина — датацентры РФ."""
    home = (n + 1) // 2
    locs = [{"country": "RU", "tags": ["eyeball-network"], "limit": home}]
    if n - home:
        locs.append({"country": "RU", "tags": ["datacenter-network"], "limit": n - home})
    return locs


def start(ip, locations, port=None):
    opts = {"packets": 3}
    if port:
        opts.update(protocol="TCP", port=port)
    body = {"type": "ping", "target": ip, "locations": locations, "measurementOptions": opts}
    return api("POST", API, body)["id"]


def wait(ids):
    out, pending, deadline = {}, set(ids), time.time() + 90
    while pending and time.time() < deadline:
        time.sleep(3)
        for mid in list(pending):
            d = api("GET", f"{API}/{mid}")
            if d.get("status") != "in-progress":
                out[mid] = d
                pending.discard(mid)
    for mid in pending:  # не дождались — берём что есть
        out[mid] = api("GET", f"{API}/{mid}")
    return out


def probes(m):
    rows = []
    for r in m.get("results", []):
        p, res = r["probe"], r["result"]
        st = res.get("stats") or {}
        ok = (st.get("rcv") or 0) > 0
        why = "" if ok else (res.get("status") if res.get("status") not in ("finished", None) else "timeout")
        kind = "дом" if "eyeball-network" in (p.get("tags") or []) else "ДЦ "
        rows.append({"cc": p.get("country"), "kind": kind, "city": p.get("city", ""), "net": p.get("network", ""),
                     "ok": ok, "loss": st.get("loss"), "why": why})
    return rows


def pct(rows):
    return round(100 * sum(r["ok"] for r in rows) / len(rows)) if rows else None


def parse_target(s, default_port):
    name = None
    parts = s.split()
    if len(parts) == 2:
        name, s = parts
    host, _, port = s.rpartition(":") if s.count(":") == 1 else (s, "", "")
    port = int(port) if port else default_port
    return name or f"{host}:{port}", host, port


def verdict(ru, fx, icmp):
    if fx is not None and fx == 0:
        return "НЕДОСТУПЕН ОТОВСЮДУ — проблема на сервере (сервис/порт/файрвол), а не ТСПУ"
    if ru is None:
        return "нет данных из РФ (зонды не ответили)"
    if ru >= 70:
        return "OK — доступен из РФ"
    if ru <= 30:
        if icmp is not None and icmp >= 70:
            return "ТСПУ РЕЖЕТ TCP — ping из РФ проходит, TCP нет (IP под блокировкой)"
        return "ЗАБЛОКИРОВАН из РФ — не проходят ни TCP, ни ping (полная блокировка IP или ICMP закрыт)"
    return "ЧАСТИЧНО — режут у части провайдеров (блокировка раскатывается или зависит от сети)"


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("targets", nargs="*", help="host[:port] (домен или IP)")
    ap.add_argument("--file", help="файл со строками 'имя host:port' или 'host:port'")
    ap.add_argument("--port", type=int, default=443, help="порт по умолчанию (443)")
    ap.add_argument("--ru", type=int, default=8, help="сколько зондов из РФ (8)")
    ap.add_argument("--no-foreign", action="store_true", help="не делать контроль из-за рубежа")
    ap.add_argument("--json", action="store_true", help="вывести сырой JSON итогов")
    a = ap.parse_args()

    raw = list(a.targets)
    if a.file:
        with open(a.file, encoding="utf-8") as f:
            raw += [l.strip() for l in f if l.strip() and not l.lstrip().startswith("#")]
    if not raw:
        ap.error("не указаны цели")

    targets = []
    for s in raw:
        name, host, port = parse_target(s, a.port)
        try:
            ip = socket.gethostbyname(host)
        except OSError as e:
            print(f"[{name}] не резолвится: {e}")
            continue
        targets.append({"name": name, "host": host, "ip": ip, "port": port})

    # Фаза 1: TCP из РФ + TCP из-за рубежа
    started = []
    for t in targets:
        try:
            t["ru_id"] = start(t["ip"], ru_locations(a.ru), t["port"])
            if not a.no_foreign:
                t["fx_id"] = start(t["ip"], [{"country": c, "limit": 1} for c in FOREIGN], t["port"])
        except QuotaError as e:
            print(f"{e}\nПроверены не все цели: пропущены начиная с {t['name']}.")
            break
        started.append(t)
    targets = started
    res = wait([t[k] for t in targets for k in ("ru_id", "fx_id") if k in t])

    # Фаза 2: ICMP из РФ — только там, где TCP из РФ плохой (экономим квоту)
    for t in targets:
        t["ru"] = probes(res[t["ru_id"]])
        t["fx"] = probes(res[t["fx_id"]]) if "fx_id" in t else []
        if (pct(t["ru"]) or 0) < 70:
            try:
                t["icmp_id"] = start(t["ip"], ru_locations(min(a.ru, 6)))
            except QuotaError:
                print(f"[{t['name']}] ping из РФ пропущен: кончилась квота")
    res2 = wait([t["icmp_id"] for t in targets if "icmp_id" in t])

    summary = []
    for t in targets:
        t["icmp"] = probes(res2[t["icmp_id"]]) if "icmp_id" in t else []
        ru, fx, ic = pct(t["ru"]), pct(t["fx"]) if t["fx"] else None, pct(t["icmp"]) if t["icmp"] else None
        t["verdict"] = verdict(ru, fx, ic)
        print(f"\n=== {t['name']}  ({t['host']} -> {t['ip']}:{t['port']})")
        for label, rows in (("РФ TCP", t["ru"]), ("Зарубеж TCP", t["fx"]), ("РФ ping", t["icmp"])):
            if not rows:
                continue
            print(f"  {label}: {sum(r['ok'] for r in rows)}/{len(rows)}")
            for r in rows:
                mark = "OK " if r["ok"] else "-- "
                print(f"    {mark}{r['cc']} {r['kind']} {r['city'][:14]:14} {r['net'][:28]:28} {r['why']}")
        print(f"  ИТОГ: {t['verdict']}")
        summary.append({"name": t["name"], "ip": t["ip"], "port": t["port"], "ru_tcp": ru,
                        "foreign_tcp": fx, "ru_icmp": ic, "verdict": t["verdict"]})

    print("\n=== Сводка (% зондов, у которых прошло)")
    print(f"  {'сервер':30} {'ip:port':22} {'РФ TCP':>7} {'зарубеж':>7} {'РФ ping':>7}  вердикт")
    fmt = lambda v: "-" if v is None else f"{v}%"
    for s in summary:
        print(f"  {s['name'][:30]:30} {s['ip'] + ':' + str(s['port']):22} {fmt(s['ru_tcp']):>7} "
              f"{fmt(s['foreign_tcp']):>7} {fmt(s['ru_icmp']):>7}  {s['verdict'].split(' —')[0]}")
    if state["remaining"] is not None:
        print(f"\nОстаток квоты Globalping: {state['remaining']}")
    if a.json:
        print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
