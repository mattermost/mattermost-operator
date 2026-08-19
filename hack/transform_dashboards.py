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
import re
import sys
from pathlib import Path

# UIDs that point at a Prometheus datasource in the upstream exports. All are
# collapsed onto one templated datasource variable.
PROM_DS_TOKENS = ("${DS_PROMETHEUS}", "${DS_PROMETHEUS_PROD}", "${DS_MATTERMOST}")
# The Grafana built-in expression datasource has a fixed UID.
EXPR_DS_TOKEN = "${DS_EXPRESSION}"
EXPR_DS_UID = "__expr__"

DATASOURCE_VAR_NAME = "datasource"

# The explicit datasource ref injected onto panels/targets that ship without one
# (older exports leaned on a single default datasource). Once every datasource is
# collapsed onto the `datasource` template var, a missing ref would fall back to
# Grafana's default and silently break the panel.
def prometheus_datasource_ref():
    return {"type": "prometheus", "uid": "${%s}" % DATASOURCE_VAR_NAME}

# Constant template variables whose resolved value is itself a Grafana built-in
# (not a literal) must be INLINED into the queries and their variable removed.
# Grafana interpolates a query in a single pass, so a constant that expands to
# "$__rate_interval" would hand Prometheus the literal built-in, unresolved.
# $__rate_interval is the recommended range for rate()/increase() — it auto-sizes
# to the scrape interval and panel range instead of the export's hardcoded value.
BUILTIN_CONSTANT_OVERRIDES = {"rate": "$__rate_interval"}


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


def constant_input_defaults(dashboard):
    """Map the export's constant __inputs (VAR_*) to their default values.

    Captured before __inputs is dropped so we can re-resolve the constant
    template variables that would otherwise be left pointing at ${VAR_*}.
    """
    return {
        i["name"]: i.get("value")
        for i in dashboard.get("__inputs", [])
        if i.get("type") == "constant" and i.get("name")
    }


def _set_constant_value(var, value):
    """Pin a constant template variable to a concrete literal value."""
    var["query"] = value
    option = {"value": value, "text": value, "selected": True}
    var["current"] = dict(option)
    var["options"] = [option]


def _inline_variable(dashboard, var_name, replacement):
    """Replace $var / ${var} tokens with a literal, everywhere in the dashboard.

    Used for constants that resolve to a Grafana built-in — the variable itself
    is dropped by the caller, so the built-in must land directly in the query.
    """
    brace = re.compile(r"\$\{%s\}" % re.escape(var_name))
    bare = re.compile(r"\$%s\b" % re.escape(var_name))

    def walk(node):
        if isinstance(node, dict):
            return {k: walk(v) for k, v in node.items()}
        if isinstance(node, list):
            return [walk(v) for v in node]
        if isinstance(node, str):
            return bare.sub(replacement, brace.sub(replacement, node))
        return node

    return walk(dashboard)


def resolve_constant_variables(dashboard, input_defaults):
    """Resolve constant template vars left holding unresolved ${VAR_*} tokens.

    Dropping __inputs strips the values the export shipped for its constant
    inputs (metrics ports, the rate interval), leaving each constant variable
    pointing at a ${VAR_NAME} placeholder that would reach Prometheus verbatim.
    Literal constants (ports) are pinned to their default and kept as variables;
    built-in intervals are inlined into the queries and their variable dropped.
    """
    templating = dashboard.get("templating", {}).get("list", [])
    survivors = []
    for var in templating:
        if var.get("type") != "constant":
            survivors.append(var)
            continue
        name = var.get("name")
        if name in BUILTIN_CONSTANT_OVERRIDES:
            dashboard = _inline_variable(dashboard, name, BUILTIN_CONSTANT_OVERRIDES[name])
            templating = dashboard["templating"]["list"]
            continue  # drop the now-orphaned variable
        token = var.get("query")
        if isinstance(token, str) and token.startswith("${") and token.endswith("}"):
            default = input_defaults.get(token[2:-1])
            if default is not None:
                _set_constant_value(var, default)
        survivors.append(var)
    dashboard["templating"]["list"] = survivors
    return dashboard


def ensure_panel_datasources(dashboard):
    """Inject the prometheus datasource ref onto panels/targets that omit one."""

    def walk_panels(panels):
        for panel in panels:
            if not isinstance(panel, dict):
                continue
            # Query panels need a datasource; layout rows do not.
            if panel.get("type") != "row" and "datasource" not in panel and "targets" in panel:
                panel["datasource"] = prometheus_datasource_ref()
            for target in panel.get("targets", []) or []:
                if isinstance(target, dict) and "datasource" not in target:
                    target["datasource"] = prometheus_datasource_ref()
            # Recurse into collapsed rows, which nest their panels.
            if isinstance(panel.get("panels"), list):
                walk_panels(panel["panels"])

    walk_panels(dashboard.get("panels", []) or [])
    return dashboard


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
    # Capture constant input defaults before __inputs is stripped — they carry
    # the port/interval values the constant template vars still reference.
    input_defaults = constant_input_defaults(dashboard)

    dashboard.pop("__inputs", None)
    dashboard.pop("__requires", None)

    dashboard = replace_ds_strings(dashboard)

    dashboard.setdefault("templating", {}).setdefault("list", [])
    dashboard = resolve_constant_variables(dashboard, input_defaults)
    templating = dashboard["templating"]["list"]

    # Does this dashboard select a Mattermost server (instance-based)? If so it
    # gets the full pod-scoping treatment.
    server_var = next((v for v in templating if v.get("name") == "server"), None)
    is_server_dashboard = server_var is not None

    if is_server_dashboard:
        podify_server_var(server_var)
        dashboard = replace_instance_filter(dashboard)
        templating = dashboard["templating"]["list"]

    # Give every query panel/target an explicit datasource ref now that the
    # single default datasource is gone.
    dashboard = ensure_panel_datasources(dashboard)

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
