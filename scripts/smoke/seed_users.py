"""Creates the six smoke-test accounts directly in MySQL and writes users.json.

Usage: SMOKE_MYSQL="mariadb -S /path/to/sock golive" python3 seed_users.py
       SMOKE_MYSQL="docker compose exec -T mysql mysql -ugolive -p$PW golive" python3 seed_users.py
Only for local or staging databases.
"""
import bcrypt, json, os, shlex, subprocess, uuid
users = {
    "admin":    {"role": "admin",     "live": "none",     "coins": 0},
    "creator":  {"role": "user",      "live": "approved", "coins": 0},
    "viewer":   {"role": "user",      "live": "none",     "coins": 1000},
    "attacker": {"role": "user",      "live": "none",     "coins": 0},
    "mod":      {"role": "moderator", "live": "none",     "coins": 0},
    "victim":   {"role": "user",      "live": "none",     "coins": 0},
}
out, sql = {}, []
for name, u in users.items():
    uid, pw = str(uuid.uuid4()), "Smoke-pass-123"
    h = bcrypt.hashpw(pw.encode(), bcrypt.gensalt(10)).decode()
    out[name] = {"id": uid, "username": f"smoke_{name}", "password": pw}
    sql.append(f"INSERT INTO users (id, username, display_name, password_hash, role, live_permission_status, "
               f"coin_balance, created_at, updated_at) VALUES ('{uid}', 'smoke_{name}', 'Smoke {name.title()}', "
               f"'{h}', '{u['role']}', '{u['live']}', {u['coins']}, NOW(), NOW());")
subprocess.run(shlex.split(os.environ["SMOKE_MYSQL"]), input="\n".join(sql), text=True, check=True)
json.dump(out, open("users.json", "w"), indent=1)
print("seeded", ", ".join(out))
