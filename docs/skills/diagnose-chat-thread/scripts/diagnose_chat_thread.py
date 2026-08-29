#!/usr/bin/env python3
"""Locate a Lumi chat thread and emit read-only diagnostic JSON."""

from __future__ import annotations

import argparse
import json
import os
import sqlite3
import sys
import uuid
from pathlib import Path
from urllib.parse import unquote, urlparse


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Locate a Lumi Chat Thread UUID and summarize its diagnostic records."
    )
    parser.add_argument("chat_thread_uuid", help="Public UUIDv7 of the Lumi chat thread")
    parser.add_argument(
        "--project-root",
        type=Path,
        help="Known Lumi project root containing project.sqlite",
    )
    parser.add_argument(
        "--app-data-dir",
        action="append",
        default=[],
        type=Path,
        help="Additional Lumi app data directory containing lumi.sqlite",
    )
    parser.add_argument(
        "--include-payloads",
        action="store_true",
        help="Include raw LLM and Tool JSON payloads; off by default",
    )
    return parser.parse_args()


def validate_uuid7(value: str) -> str:
    try:
        parsed = uuid.UUID(value.strip())
    except ValueError as error:
        raise ValueError("chat_thread_uuid must be a valid UUIDv7") from error
    if parsed.version != 7:
        raise ValueError("chat_thread_uuid must be a UUIDv7")
    return str(parsed)


def readonly_connection(path: Path) -> sqlite3.Connection:
    uri = "file:" + path.resolve().as_posix() + "?mode=ro"
    connection = sqlite3.connect(uri, uri=True)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA query_only=ON")
    connection.execute("PRAGMA busy_timeout=1000")
    return connection


def rows(connection: sqlite3.Connection, query: str, parameters: tuple = ()) -> list[dict]:
    return [dict(row) for row in connection.execute(query, parameters).fetchall()]


def has_table(connection: sqlite3.Connection, table: str) -> bool:
    result = connection.execute(
        "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (table,)
    ).fetchone()
    return result is not None


def database_path_from_dsn(raw: str) -> Path | None:
    raw = raw.strip()
    if not raw.startswith("file:"):
        return None
    parsed = urlparse(raw)
    path = unquote(parsed.path)
    return Path(path) if path else None


def unique_paths(paths: list[Path]) -> list[Path]:
    result: list[Path] = []
    seen: set[str] = set()
    for path in paths:
        expanded = path.expanduser()
        key = str(expanded.resolve(strict=False))
        if key not in seen:
            result.append(expanded)
            seen.add(key)
    return result


def app_database_candidates(extra_dirs: list[Path]) -> list[Path]:
    candidates = [directory / "lumi.sqlite" for directory in extra_dirs]
    data_dir = os.environ.get("LUMI_DATA_DIR", "").strip()
    if data_dir:
        candidates.append(Path(data_dir) / "lumi.sqlite")
    dsn_path = database_path_from_dsn(os.environ.get("DATABASE_DSN", ""))
    if dsn_path is not None:
        candidates.append(dsn_path)
    home = Path.home()
    environment = os.environ.get("APP_ENV", "development").strip().lower()
    if environment == "production":
        candidates.extend([home / ".lumi/lumi.sqlite", home / ".lumi_dev/lumi.sqlite"])
    else:
        candidates.extend([home / ".lumi_dev/lumi.sqlite", home / ".lumi/lumi.sqlite"])
    return unique_paths(candidates)


def indexed_projects(app_databases: list[Path]) -> list[dict]:
    projects: list[dict] = []
    for app_database in app_databases:
        if not app_database.is_file():
            continue
        try:
            with readonly_connection(app_database) as connection:
                if not has_table(connection, "recent_projects"):
                    continue
                for project in rows(
                    connection,
                    """
                    SELECT uuid AS project_uuid,name AS project_name,root_path,last_opened_at
                    FROM recent_projects
                    ORDER BY last_opened_at DESC,id DESC
                    """,
                ):
                    project["app_database"] = str(app_database.resolve())
                    projects.append(project)
        except sqlite3.Error:
            continue
    return projects


def project_candidates(args: argparse.Namespace) -> list[dict]:
    if args.project_root is not None:
        return [{"root_path": str(args.project_root.expanduser().resolve())}]

    candidates = indexed_projects(app_database_candidates(args.app_data_dir))
    default_parent = Path.home() / "Documents/Lumi"
    if default_parent.is_dir():
        known_roots = {
            str(Path(item["root_path"]).expanduser().resolve(strict=False))
            for item in candidates
        }
        for database in sorted(default_parent.glob("*/project.sqlite")):
            root = str(database.parent.resolve())
            if root not in known_roots:
                candidates.append({"root_path": root, "app_database": ""})
                known_roots.add(root)
    return candidates


def parse_json(value: str | None):
    if value is None or value == "":
        return None
    try:
        return json.loads(value)
    except json.JSONDecodeError:
        return value


def tool_api_error(result):
    if not isinstance(result, dict) or result.get("success") is not False:
        return None
    error = result.get("error")
    return error if isinstance(error, dict) else {"message": "API returned success:false"}


def notable_event(event_type: str) -> bool:
    lowered = event_type.lower()
    return any(
        word in lowered
        for word in ("failed", "error", "budget", "cancel", "interrupt", "exhaust")
    )


def diagnose(database: Path, thread_uuid: str, include_payloads: bool) -> dict | None:
    with readonly_connection(database) as connection:
        if not has_table(connection, "chat_threads"):
            return None
        thread_rows = rows(
            connection,
            """
            SELECT p.uuid AS project_uuid,p.name AS project_name,
                   t.uuid AS thread_uuid,t.title,t.status,t.thread_type,
                   t.provider_uuid,t.model,t.model_source,t.archived_at,
                   t.created_at,t.updated_at
            FROM chat_threads AS t
            JOIN projects AS p ON p.id=t.project_id
            WHERE t.uuid=?
            LIMIT 2
            """,
            (thread_uuid,),
        )
        if not thread_rows:
            return None
        if len(thread_rows) != 1:
            raise RuntimeError("project database contains duplicate chat thread UUIDs")

        run_rows = rows(
            connection,
            """
            SELECT r.uuid AS run_uuid,turns.uuid AS turn_uuid,r.trigger_type,r.status,
                   r.step_count,r.max_steps,r.provider_uuid,r.model,r.model_source,
                   r.model_request_count,r.max_model_requests,
                   r.active_duration_ms,r.max_active_duration_ms,
                   r.token_units,r.max_token_units,
                   r.no_progress_streak,r.max_no_progress_rounds,r.limit_reason,
                   r.error_code,r.error_message,r.started_at,r.completed_at,r.created_at
            FROM chat_runs AS r
            JOIN chat_turns AS turns ON turns.id=r.turn_id
            JOIN chat_threads AS threads ON threads.id=r.thread_id
            WHERE threads.uuid=?
            ORDER BY r.created_at,r.id
            """,
            (thread_uuid,),
        )

        event_rows = rows(
            connection,
            """
            SELECT e.uuid,e.sequence,e.event_type,e.payload_json,e.created_at
            FROM chat_events AS e
            JOIN chat_threads AS threads ON threads.id=e.thread_id
            WHERE threads.uuid=?
            ORDER BY e.sequence,e.id
            """,
            (thread_uuid,),
        )
        for event in event_rows:
            event["payload"] = parse_json(event.pop("payload_json"))
        event_rows = [event for event in event_rows if notable_event(event["event_type"])]

        tool_rows = rows(
            connection,
            """
            SELECT executions.uuid AS tool_execution_uuid,runs.uuid AS run_uuid,
                   turns.uuid AS turn_uuid,executions.tool_call_uuid,
                   executions.tool_name,executions.target_uuid,
                   executions.idempotency_key,executions.state,
                   executions.arguments_json,executions.result_json,
                   executions.error_code,executions.error_message,
                   executions.started_at,executions.completed_at,executions.created_at
            FROM agent_tool_executions AS executions
            JOIN chat_runs AS runs ON runs.id=executions.run_id
            JOIN chat_turns AS turns ON turns.id=executions.turn_id
            JOIN chat_threads AS threads ON threads.id=executions.thread_id
            WHERE threads.uuid=?
            ORDER BY executions.created_at,executions.id
            """,
            (thread_uuid,),
        )
        for tool in tool_rows:
            arguments = parse_json(tool.pop("arguments_json"))
            result = parse_json(tool.pop("result_json"))
            if isinstance(arguments, dict):
                tool["request_ordinal"] = arguments.get("__request_ordinal")
                tool["method"] = arguments.get("method") or arguments.get("__method")
                tool["url"] = arguments.get("url") or arguments.get("__path")
            tool["api_error"] = tool_api_error(result)
            if include_payloads:
                tool["arguments"] = arguments
                tool["result"] = result

        payload_columns = ",logs.request_payload,logs.response" if include_payloads else ""
        model_rows = rows(
            connection,
            f"""
            SELECT logs.uuid AS model_request_uuid,runs.uuid AS run_uuid,
                   turns.uuid AS turn_uuid,logs.attempt AS request_ordinal,
                   logs.scenario,logs.request_type,logs.provider_uuid,
                   logs.provider_type,logs.model,logs.status,
                   logs.input_summary,logs.output_summary,
                   logs.input_tokens,logs.cached_input_tokens,logs.output_tokens,
                   logs.duration_ms,logs.finish_reason,logs.error_code,
                   logs.error_message,logs.http_status,logs.provider_error_code,
                   logs.provider_request_id,logs.created_at,logs.completed_at
                   {payload_columns}
            FROM llm_logs AS logs
            JOIN chat_runs AS runs ON runs.id=logs.chat_run_id
            JOIN chat_turns AS turns ON turns.id=runs.turn_id
            JOIN chat_threads AS threads ON threads.id=logs.chat_thread_id
            WHERE threads.uuid=? AND logs.source_type='project_chat'
            ORDER BY logs.created_at,logs.id
            """,
            (thread_uuid,),
        )
        if include_payloads:
            for model_request in model_rows:
                model_request["request_payload"] = parse_json(model_request["request_payload"])
                model_request["response"] = parse_json(model_request["response"])

        observations = []
        for tool in tool_rows:
            if tool.get("api_error") or tool.get("error_code"):
                observations.append(
                    {
                        "kind": "tool_api_error",
                        "run_uuid": tool.get("run_uuid"),
                        "tool_execution_uuid": tool.get("tool_execution_uuid"),
                        "tool_name": tool.get("tool_name"),
                        "api_error": tool.get("api_error"),
                        "error_code": tool.get("error_code"),
                    }
                )
        for run in run_rows:
            if run.get("error_code") or run.get("limit_reason"):
                observations.append(
                    {
                        "kind": "run_failure_or_limit",
                        "run_uuid": run.get("run_uuid"),
                        "status": run.get("status"),
                        "error_code": run.get("error_code"),
                        "limit_reason": run.get("limit_reason"),
                        "no_progress_streak": run.get("no_progress_streak"),
                    }
                )
        for request in model_rows:
            if request.get("status") == "failed" or request.get("error_code"):
                observations.append(
                    {
                        "kind": "model_request_error",
                        "run_uuid": request.get("run_uuid"),
                        "model_request_uuid": request.get("model_request_uuid"),
                        "error_code": request.get("error_code"),
                        "provider_error_code": request.get("provider_error_code"),
                        "http_status": request.get("http_status"),
                    }
                )

        return {
            "thread": thread_rows[0],
            "runs": run_rows,
            "notable_events": event_rows,
            "tools": tool_rows,
            "model_requests": model_rows,
            "observations": observations,
        }


def main() -> int:
    args = parse_args()
    try:
        thread_uuid = validate_uuid7(args.chat_thread_uuid)
        matches = []
        checked_databases = []
        for candidate in project_candidates(args):
            root = Path(candidate["root_path"]).expanduser()
            database = root / "project.sqlite"
            if not database.is_file():
                continue
            checked_databases.append(str(database.resolve()))
            try:
                diagnostic = diagnose(database, thread_uuid, args.include_payloads)
            except sqlite3.Error:
                continue
            if diagnostic is None:
                continue
            diagnostic["location"] = {
                "root_path": str(root.resolve()),
                "database_path": str(database.resolve()),
                "app_database": candidate.get("app_database", ""),
            }
            matches.append(diagnostic)

        if not matches:
            output = {
                "success": False,
                "data": None,
                "error": {
                    "code": "chat_thread_not_found",
                    "message": "Chat Thread UUID was not found in indexed Lumi projects.",
                    "checked_databases": checked_databases,
                },
            }
            print(json.dumps(output, ensure_ascii=False, indent=2))
            return 2
        if len(matches) > 1:
            raise RuntimeError("chat_thread_uuid matched more than one project database")

        print(json.dumps({"success": True, "data": matches[0]}, ensure_ascii=False, indent=2))
        return 0
    except (ValueError, RuntimeError) as error:
        print(
            json.dumps(
                {
                    "success": False,
                    "data": None,
                    "error": {"code": "diagnostic_failed", "message": str(error)},
                },
                ensure_ascii=False,
                indent=2,
            )
        )
        return 1


if __name__ == "__main__":
    sys.exit(main())
