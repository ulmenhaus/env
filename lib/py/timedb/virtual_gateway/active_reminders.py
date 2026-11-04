import datetime

from timedb import schema
from timedb.virtual_gateway import common, relative_utils
from timedb import client_utils

from jql import jql_pb2, jql_pb2_grpc

# Status is an enum; Next Step, Link, Parent Notes, Updated, Time-Estimate are plain text.
VALUES = {
    "Status": common.STATUS_VALUES,
}

# Maps display column names that differ from assertion relation names.
_DISPLAY_TO_RELATION = {
    "Next Step": "NextStep",
    "Parent Notes": "Note",
    "Time-Estimate": "TimeEstimate",
}

_TIMESTAMP_FORMAT = "%Y-%m-%d %H:%M:%S"

_TIME_ESTIMATE_INCREMENT_MINUTES = 5


def _format_now():
    return datetime.datetime.now().strftime(_TIMESTAMP_FORMAT)


def _parse_time_estimate_minutes(value):
    if not value:
        return None
    minutes = value[:-1] if value.endswith("m") else value
    try:
        return int(minutes)
    except ValueError:
        return None


def _is_awaiting(status_raw, updated, time_estimate):
    if status_raw != "Ready":
        return False
    minutes = _parse_time_estimate_minutes(time_estimate)
    if not updated or minutes is None:
        return False
    try:
        updated_at = datetime.datetime.strptime(updated, _TIMESTAMP_FORMAT)
    except ValueError:
        return False
    return datetime.datetime.now() < updated_at + datetime.timedelta(minutes=minutes)


class ActiveRemindersBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        # Query 1: find today's day plan
        plan_pk = client_utils.get_todays_day_plan(self.client)
        # Query 2: get reminder arg0s and dividers that are in the day plan
        entry_refs, dividers = client_utils.get_day_plan_entry_refs(
            self.client, plan_pk, include_dividers=True)
        reminder_bareids = list(entry_refs.keys())
        if not reminder_bareids and not dividers:
            return common.list_rows("vt.active_reminders", {}, request, VALUES)

        entries = {}

        if reminder_bareids:
            # Query 3: get all reminder attributes
            attr_map, raw_assn_pks = client_utils.get_fields_for_items_by_pk(
                self.client, "vt.reminders", reminder_bareids)

            # Collect task PKs so we can batch-query their notes
            task_pks = []
            for bareid in reminder_bareids:
                task_val = attr_map.get(bareid, {}).get("Task", [""])[0]
                if task_val and client_utils.is_foreign(task_val):
                    _, task_pk = client_utils.parse_foreign(task_val)
                    task_pks.append(task_pk)

            # Query 4: get .Note attributes for all referenced tasks
            task_assn_pks = {}
            if task_pks:
                _, task_assn_pks = client_utils.get_fields_for_items_by_pk(
                    self.client, "tasks", task_pks, fields=("Note",))

            for bareid in reminder_bareids:
                fields = attr_map.get(bareid, {})
                field_assn_pks = dict(raw_assn_pks.get(bareid, {}))

                # Remap assertion relation names to display column names
                if "NextStep" in field_assn_pks:
                    field_assn_pks["Next Step"] = field_assn_pks.pop("NextStep")
                if "TimeEstimate" in field_assn_pks:
                    field_assn_pks["Time-Estimate"] = field_assn_pks.pop("TimeEstimate")

                # Encode entry assertion PK and order for Order column updates
                entry_assn_pk, entry_order = entry_refs.get(bareid, ("", "0"))
                field_assn_pks["A Order"] = [[entry_assn_pk, entry_order]]

                task_val = fields.get("Task", [""])[0]
                check_val = fields.get("Check", [""])[0]
                description = check_val if check_val else task_val

                # Gather task notes and encode references for WriteRow
                notes_pairs = []
                task_pk = ""
                if task_val and client_utils.is_foreign(task_val):
                    _, task_pk = client_utils.parse_foreign(task_val)
                    notes_pairs = list(task_assn_pks.get(task_pk, {}).get("Note", []))

                note_values = [v for _, v in notes_pairs]
                if len(note_values) == 0:
                    parent_notes_display = []
                elif len(note_values) == 1:
                    parent_notes_display = [note_values[0]]
                else:
                    parent_notes_display = ["\n".join(f"* {v}" for v in note_values)]

                # Encode task bareid and note assn pairs into pk_map for WriteRow
                field_assn_pks["Parent Notes"] = notes_pairs
                if task_pk:
                    field_assn_pks["Task"] = [["", f"tasks {task_pk}"]]

                status_raw = fields.get("Status", [""])[0]
                updated = fields.get("Updated", [""])[0]
                time_estimate = fields.get("TimeEstimate", [""])[0]
                display_status = "Awaiting" if _is_awaiting(
                    status_raw, updated, time_estimate) else status_raw
                pk = client_utils.encode_pk(f"vt.reminders {bareid}", field_assn_pks)
                entries[pk] = {
                    "_pk": [pk],
                    "Description": [description] if description else [],
                    "Status": [common.colorize_status(display_status)] if status_raw else [],
                    "Next Step": fields.get("NextStep", []),
                    "Link": fields.get("Link", []),
                    "Parent Notes": parent_notes_display,
                    "Updated": fields.get("Updated", []),
                    "Time-Estimate": fields.get("TimeEstimate", []),
                    "A Order": [entry_order],
                }

        for assn_pk, order, text in dividers:
            divider_pk = client_utils.encode_pk(
                f"divider {assn_pk}", {"A Order": [[assn_pk, order]]})
            entries[divider_pk] = {
                "_pk": [divider_pk],
                "Description": [f"\033[1m{text}\033[0m"],
                "Status": ["-"],
                "A Order": [order],
            }

        return common.list_rows("vt.active_reminders", entries, request, VALUES)

    def GetRow(self, request, context):
        arg0, _ = client_utils.decode_pk(request.pk)
        resp = self.ListRows(jql_pb2.ListRowsRequest(), context)
        primary, _ = client_utils.list_rows_meta(resp)
        for row in resp.rows:
            row_arg0, _ = client_utils.decode_pk(row.entries[primary].formatted)
            if row_arg0 == arg0:
                return jql_pb2.GetRowResponse(
                    table="vt.active_reminders",
                    columns=resp.columns,
                    row=row,
                )
        raise ValueError(request.pk)

    def IncrementEntry(self, request, context):
        arg0, pk_map = client_utils.decode_pk(request.pk)
        if arg0.startswith("divider ") and request.column != "A Order":
            raise ValueError("Cannot increment column", request.column)

        if request.column == "Status":
            if "Status" in pk_map:
                assn_pk, current = pk_map["Status"][0]
                values = common.STATUS_VALUES
                current_index = values.index(current) if current in values else 0
                next_value = values[(current_index + request.amount) % len(values)]
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                    fields={schema.Fields.Arg1: next_value},
                    update_only=True,
                ))
            else:
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((".Status", arg0, "0000")),
                    fields={
                        schema.Fields.Relation: ".Status",
                        schema.Fields.Arg0: arg0,
                        schema.Fields.Arg1: common.STATUS_VALUES[0],
                    },
                    insert_only=True,
                ))
        elif request.column == "Updated":
            now = _format_now()
            if "Updated" in pk_map:
                assn_pk, _ = pk_map["Updated"][0]
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                    fields={schema.Fields.Arg1: now},
                    update_only=True,
                ))
            else:
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((".Updated", arg0, "0000")),
                    fields={
                        schema.Fields.Relation: ".Updated",
                        schema.Fields.Arg0: arg0,
                        schema.Fields.Arg1: now,
                    },
                    insert_only=True,
                ))
        elif request.column == "Time-Estimate":
            if "Time-Estimate" in pk_map:
                assn_pk, current = pk_map["Time-Estimate"][0]
                current_minutes = _parse_time_estimate_minutes(current) or 0
                new_minutes = max(
                    0, current_minutes +
                    _TIME_ESTIMATE_INCREMENT_MINUTES * request.amount)
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                    fields={schema.Fields.Arg1: f"{new_minutes}m"},
                    update_only=True,
                ))
            else:
                new_minutes = max(
                    0, _TIME_ESTIMATE_INCREMENT_MINUTES * request.amount)
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((".TimeEstimate", arg0, "0000")),
                    fields={
                        schema.Fields.Relation: ".TimeEstimate",
                        schema.Fields.Arg0: arg0,
                        schema.Fields.Arg1: f"{new_minutes}m",
                    },
                    insert_only=True,
                ))
        elif request.column == "Next Step":
            prefix = f"- {_format_now()}"
            if "Next Step" in pk_map:
                assn_pk, current = pk_map["Next Step"][0]
                new_value = f"{prefix}\n{current}" if current else prefix
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                    fields={schema.Fields.Arg1: new_value},
                    update_only=True,
                ))
            else:
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((".NextStep", arg0, "0000")),
                    fields={
                        schema.Fields.Relation: ".NextStep",
                        schema.Fields.Arg0: arg0,
                        schema.Fields.Arg1: prefix,
                    },
                    insert_only=True,
                ))
        elif request.column == "A Order":
            if "A Order" in pk_map:
                assn_pk, current = pk_map["A Order"][0]
                try:
                    new_order = int(current) + request.amount
                except (ValueError, TypeError):
                    new_order = request.amount
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                    fields={schema.Fields.Order: str(new_order)},
                    update_only=True,
                ))
        else:
            raise ValueError("Cannot increment column", request.column)

        return jql_pb2.IncrementEntryResponse()

    def WriteRow(self, request, context):
        arg0, pk_map = client_utils.decode_pk(request.pk)
        if arg0.startswith("divider "):
            for field in request.fields:
                if field != "A Order":
                    raise ValueError("Cannot write column", field)

        for field, value in request.fields.items():
            if field == "Parent Notes":
                task_arg0 = pk_map["Task"][0][1]
                relative_utils.update_attrs(
                    self.client,
                    task_arg0,
                    {"Note": pk_map.get("Parent Notes", [])},
                    {"Note": value},
                )
            elif field == "Status":
                relation = ".Status"
                if "Status" in pk_map:
                    assn_pk, _ = pk_map["Status"][0]
                    self.client.WriteRow(jql_pb2.WriteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=assn_pk,
                        fields={schema.Fields.Arg1: value},
                        update_only=True,
                    ))
                else:
                    self.client.WriteRow(jql_pb2.WriteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=str((relation, arg0, "0000")),
                        fields={
                            schema.Fields.Relation: relation,
                            schema.Fields.Arg0: arg0,
                            schema.Fields.Arg1: value,
                        },
                        insert_only=True,
                    ))
            elif field == "Updated":
                if value != "":
                    raise ValueError(
                        "Updated can only be cleared via WriteRow; use IncrementEntry to set it",
                        value)
                if "Updated" in pk_map:
                    assn_pk, _ = pk_map["Updated"][0]
                    self.client.DeleteRow(jql_pb2.DeleteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=assn_pk,
                    ))
            elif field in ("Next Step", "Link", "Time-Estimate"):
                relation = f".{_DISPLAY_TO_RELATION.get(field, field)}"
                if field in pk_map:
                    assn_pk, _ = pk_map[field][0]
                    self.client.WriteRow(jql_pb2.WriteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=assn_pk,
                        fields={schema.Fields.Arg1: value},
                        update_only=True,
                    ))
                else:
                    self.client.WriteRow(jql_pb2.WriteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=str((relation, arg0, "0000")),
                        fields={
                            schema.Fields.Relation: relation,
                            schema.Fields.Arg0: arg0,
                            schema.Fields.Arg1: value,
                        },
                        insert_only=True,
                    ))
            elif field == "A Order":
                if "A Order" in pk_map:
                    assn_pk, _ = pk_map["A Order"][0]
                    self.client.WriteRow(jql_pb2.WriteRowRequest(
                        table=schema.Tables.Assertions,
                        pk=assn_pk,
                        fields={schema.Fields.Order: value},
                        update_only=True,
                    ))
            else:
                raise ValueError("Cannot write column", field)

        return jql_pb2.WriteRowResponse()
