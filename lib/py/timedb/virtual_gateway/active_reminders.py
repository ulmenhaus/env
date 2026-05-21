from timedb import schema
from timedb.virtual_gateway import common, relative_utils

from jql import jql_pb2, jql_pb2_grpc

# Status is an enum; Next Step, Link, Parent Notes are plain text.
VALUES = {
    "Status": common.STATUS_VALUES,
}

# Maps display column names that differ from assertion relation names.
_DISPLAY_TO_RELATION = {
    "Next Step": "NextStep",
    "Parent Notes": "Note",
}


class ActiveRemindersBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        # Query 1: find today's day plan
        plan_pk = common.get_todays_day_plan(self.client)
        # Query 2: get reminder arg0s that are in the day plan
        entry_refs = common.get_day_plan_entry_refs(self.client, plan_pk)
        reminder_arg0s = list(entry_refs.keys())
        if not reminder_arg0s:
            return common.list_rows("vt.active_reminders", {}, request, VALUES)

        # Query 3: get all reminder attributes
        attr_map, raw_assn_pks = common.get_fields_for_items(
            self.client, "", reminder_arg0s)

        # Collect task PKs so we can batch-query their notes
        task_pks = []
        for arg0 in reminder_arg0s:
            task_val = attr_map.get(arg0, {}).get("Task", [""])[0]
            if task_val and common.is_foreign(task_val):
                _, task_pk = common.parse_foreign(task_val)
                task_pks.append(task_pk)

        # Query 4: get .Note attributes for all referenced tasks
        task_assn_pks = {}
        if task_pks:
            _, task_assn_pks = common.get_fields_for_items(
                self.client, "tasks", task_pks, fields=("Note",))

        entries = {}
        for arg0 in reminder_arg0s:
            fields = attr_map.get(arg0, {})
            field_assn_pks = dict(raw_assn_pks.get(arg0, {}))

            # Remap assertion relation names to display column names
            if "NextStep" in field_assn_pks:
                field_assn_pks["Next Step"] = field_assn_pks.pop("NextStep")

            task_val = fields.get("Task", [""])[0]
            check_val = fields.get("Check", [""])[0]
            description = check_val if check_val else task_val

            # Gather task notes and encode references for WriteRow
            notes_pairs = []
            task_pk = ""
            if task_val and common.is_foreign(task_val):
                _, task_pk = common.parse_foreign(task_val)
                notes_pairs = list(task_assn_pks.get(task_pk, {}).get("Note", []))

            note_values = [v for _, v in notes_pairs]
            if len(note_values) == 0:
                parent_notes_display = []
            elif len(note_values) == 1:
                parent_notes_display = [note_values[0]]
            else:
                parent_notes_display = ["\n".join(f"* {v}" for v in note_values)]

            # Encode task arg0 and note assn pairs into pk_map for WriteRow
            field_assn_pks["Parent Notes"] = notes_pairs
            if task_pk:
                field_assn_pks["Task"] = [["", f"tasks {task_pk}"]]

            status_raw = fields.get("Status", [""])[0]
            pk = common.encode_pk(arg0, field_assn_pks)
            entries[pk] = {
                "_pk": [pk],
                "Description": [description] if description else [],
                "Status": [common.colorize_status(status_raw)] if status_raw else [],
                "Next Step": fields.get("NextStep", []),
                "Link": fields.get("Link", []),
                "Parent Notes": parent_notes_display,
            }

        return common.list_rows("vt.active_reminders", entries, request, VALUES)

    def IncrementEntry(self, request, context):
        arg0, pk_map = common.decode_pk(request.pk)

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
        else:
            raise ValueError("Cannot increment column", request.column)

        return jql_pb2.IncrementEntryResponse()

    def WriteRow(self, request, context):
        arg0, pk_map = common.decode_pk(request.pk)

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
            elif field in ("Next Step", "Link"):
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
            else:
                raise ValueError("Cannot write column", field)

        return jql_pb2.WriteRowResponse()
