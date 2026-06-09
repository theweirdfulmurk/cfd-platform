#!/usr/bin/env python3
"""Rebuild the backend run-index (/pvc/simulations/_index.json) from the saved
benchmark CSVs after the in-memory store was lost on a backend restart.

Only rows with a non-empty sim_id are real, created runs (errored cells never
got an ID). Field mapping mirrors domain.Simulation / the frontend:
  SchedulerName = csv scheduler verbatim  (algorithmTag routes on this string:
    random-scheduler→Быстрый, topology-aware→Сбалансированный, mueller-merbach→Точный)
  CompletedAt   = CreatedAt + wall_time_s ; StartedAt = CreatedAt (so the UI
    duration shows the measured wall time).

Usage: seed_index_from_csv.py OUT.json CSV [CSV ...]
"""
import csv, json, sys
from datetime import datetime, timedelta

ALGO = {"random-scheduler": "random", "topology-aware": "greedy",
        "mueller-merbach": "mueller-merbach"}
TERMINAL = {"completed": "completed", "error": "failed", "failed": "failed"}


def iso(dt):
    return dt.strftime("%Y-%m-%dT%H:%M:%SZ")


def main():
    out, csvs = sys.argv[1], sys.argv[2:]
    rec = {}
    for path in csvs:
        with open(path) as f:
            for row in csv.DictReader(f):
                sid = (row.get("sim_id") or "").strip()
                if not sid:
                    continue
                solver = row["solver"].strip()
                sched = row["scheduler"].strip()
                np_ = int(row["np"])
                rep = int(row["rep"])
                status = TERMINAL.get(row["status"].strip(), "failed")
                created = datetime.strptime(row["timestamp"].strip(),
                                            "%Y-%m-%dT%H:%M:%S")
                wall = float(row["wall_time_s"]) if row.get("wall_time_s") else 0.0
                completed = created + timedelta(seconds=wall)
                rec[sid] = {
                    "ID": sid,
                    "Name": f"bench-{solver}-{sched}-n{np_}-r{rep:02d}",
                    "Type": solver,
                    "Status": status,
                    "NumProcs": np_,
                    "SchedulerName": sched,
                    "Algorithm": ALGO.get(sched, ""),
                    "PodName": f"sim-{sid}",
                    "ResultPath": f"results/{sid}",
                    "ConfigPath": sid,
                    "CreatedAt": iso(created),
                    "StartedAt": iso(created),
                    "CompletedAt": iso(completed) if status == "completed" else None,
                    "Zones": (row.get("zones") or "").strip(),
                }
    with open(out, "w") as f:
        json.dump(rec, f, indent=2)
    print(f"wrote {len(rec)} records -> {out}")


if __name__ == "__main__":
    main()
