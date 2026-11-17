import sys
import paramiko

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
passw = open(r"C:\Users\Michail\Downloads\inf.txt", encoding="utf-8").read().strip().splitlines()[2].strip()
c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect(
    "169.128.68.61",
    username="root",
    password=passw,
    timeout=30,
    allow_agent=False,
    look_for_keys=False,
    banner_timeout=45,
)
cmds = [
    "docker logs remnawave-telegram-shop-bot --since 72h 2>&1 | tr -cd '\\11\\12\\15\\40-\\176\\n' | grep -iE 'fortune|admin/settings|settings.*400|invalid|FORTUNE_' | tail -60",
    """docker exec remnawave-telegram-shop-db psql -U postgres -c "SELECT key, left(value,80) AS value FROM bot_runtime_settings WHERE key LIKE 'FORTUNE_%' ORDER BY key;" """,
]
for cmd in cmds:
    print("=====")
    stdin, stdout, stderr = c.exec_command(cmd, timeout=90)
    print(stdout.read().decode("utf-8", "replace")[-8000:])
    err = stderr.read().decode("utf-8", "replace")
    if err.strip():
        print("ERR:", err[-500:])
c.close()
print("DONE")
