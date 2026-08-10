#!/usr/bin/env python3
"""Transform upstream Mattermost Grafana dashboards for operator/kube-prometheus use.

The dashboards published in mattermost-performance-assets are portable Grafana
exports built for instance/IP-based Prometheus targets. Two fixes make them work
when the operator's ServiceMonitor scrapes Mattermost in Kubernetes:

  1. Datasource normalization — replace the per-export ${DS_*} input UIDs with a
     single `datasource` template variable and drop the __inputs/__requires
     blocks, so the Grafana sidecar can provision them without an import prompt.

  2. instance -> pod — dashboards select a server by the raw `instance` label
     (podIP:8067, churns on restart). Rewrite the `server` variable to pick the
     stable `pod` label (scoped by a new `namespace` variable) and rewrite every
     `instance=~"$server"` query filter to `pod=~"$server"`.

Usage: transform_dashboards.py <src_dir> <dst_dir>

Note: the calls dashboard's rtcd/offloader/node-exporter (:9100) targets and the
client-app dashboards' agent/platform selectors are left as-is — they depend on
metrics only present when the matching flag/plugin/exporter is running.
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

# UIDs that point at a Prometheus datasource in the upstream exports. All are
# collapsed onto one templated datasource variable.
PROM_DS_TOKENS = ("${DS_PROMETHEUS}", "${DS_PROMETHEUS_PROD}", "${DS_MATTERMOST}")
# The Grafana built-in expression datasource has a fixed UID.
EXPR_DS_TOKEN = "${DS_EXPRESSION}"
EXPR_DS_UID = "__expr__"

DATASOURCE_VAR_NAME = "datasource"


def replace_ds_strings(node):
    """Recursively rewrite datasource UID string tokens in place."""
    if isinstance(node, dict):
        return {k: replace_ds_strings(v) for k, v in node.items()}
    if isinstance(node, list):
        return [replace_ds_strings(v) for v in node]
    if isinstance(node, str):
        if node in PROM_DS_TOKENS:
            return "${%s}" % DATASOURCE_VAR_NAME
        if node == EXPR_DS_TOKEN:
            return EXPR_DS_UID
        return node
    return node


def replace_instance_filter(node):
    """Recursively rewrite instance=~"$server" -> pod=~"$server" in query strings."""
    if isinstance(node, dict):
        return {k: replace_instance_filter(v) for k, v in node.items()}
    if isinstance(node, list):
        return [replace_instance_filter(v) for v in node]
    if isinstance(node, str):
        return node.replace('instance=~"$server"', 'pod=~"$server"')
    return node


def datasource_variable():
    return {
        "name": DATASOURCE_VAR_NAME,
        "label": "Datasource",
        "type": "datasource",
        "query": "prometheus",
        "refresh": 1,
        "hide": 0,
        "current": {},
        "options": [],
        "includeAll": False,
        "multi": False,
    }


def namespace_variable():
    # Namespace picker scoped to installs that expose Mattermost server metrics.
    return {
        "name": "namespace",
        "label": "Namespace",
        "type": "query",
        "datasource": {"type": "prometheus", "uid": "${%s}" % DATASOURCE_VAR_NAME},
        "query": {
            "query": "label_values(mattermost_system_server_start_time, namespace)",
            "refId": "namespace",
        },
        "definition": "label_values(mattermost_system_server_start_time, namespace)",
        "refresh": 2,
        "hide": 0,
        "sort": 1,
        "current": {},
        "options": [],
        "includeAll": False,
        "multi": False,
    }


def podify_server_var(var):
    """Rewrite the `server` variable to select the pod label instead of instance."""
    var["datasource"] = {"type": "prometheus", "uid": "${%s}" % DATASOURCE_VAR_NAME}
    query = "label_values(mattermost_system_server_start_time{namespace=\"$namespace\"}, pod)"
    if isinstance(var.get("query"), dict):
        var["query"]["query"] = query
    else:
        var["query"] = query
    var["definition"] = query
    var["refresh"] = 2
    var.setdefault("sort", 1)
    # Clear any baked-in instance values from the export.
    var["current"] = {}
    var["options"] = []
    return var


def transform(dashboard):
    dashboard.pop("__inputs", None)
    dashboard.pop("__requires", None)

    dashboard = replace_ds_strings(dashboard)

    templating = dashboard.setdefault("templating", {}).setdefault("list", [])

    # Does this dashboard select a Mattermost server (instance-based)? If so it
    # gets the full pod-scoping treatment.
    server_var = next((v for v in templating if v.get("name") == "server"), None)
    is_server_dashboard = server_var is not None

    if is_server_dashboard:
        podify_server_var(server_var)
        dashboard = replace_instance_filter(dashboard)
        templating = dashboard["templating"]["list"]

    # Prepend datasource (+ namespace for server dashboards) so they render first.
    leading = [datasource_variable()]
    if is_server_dashboard and not any(v.get("name") == "namespace" for v in templating):
        leading.append(namespace_variable())
    dashboard["templating"]["list"] = leading + templating

    return dashboard


def main():
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(2)
    src, dst = Path(sys.argv[1]), Path(sys.argv[2])
    dst.mkdir(parents=True, exist_ok=True)

    for path in sorted(src.glob("*.json")):
        with path.open() as fh:
            dashboard = json.load(fh)
        out = transform(dashboard)
        with (dst / path.name).open("w") as fh:
            json.dump(out, fh, indent=2)
            fh.write("\n")
        print(f"transformed {path.name}")


if __name__ == "__main__":
    main()
