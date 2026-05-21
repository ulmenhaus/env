import datetime

from timedb import schema
from timedb.virtual_gateway import common

from jql import jql_pb2, jql_pb2_grpc

VALUES = {
    "Active": ["Later", "Today"],
    "Status": common.STATUS_VALUES,
    "Priority": ["p0", "p1", "p2", "p3", "p4"],
}

_DATE_FORMAT = "%Y-%m-%d"


class RemindersBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        # Query 1: find all reminder identifiers via their .Status assertions
        status_response = self.client.ListRows(jql_pb2.ListRowsRequest(
            table=schema.Tables.Assertions,
            conditions=[jql_pb2.Condition(requires=[
                jql_pb2.Filter(
                    column=schema.Fields.Relation,
                    equal_match=jql_pb2.EqualMatch(value=".Status"),
                ),
                jql_pb2.Filter(
                    column=schema.Fields.Arg1,
                    in_match=jql_pb2.InMatch(values=VALUES["Status"]),
                ),
            ])],
        ))
        _, cmap = common.list_rows_meta(status_response)
        # Deduplicate while preserving order in case of malformed data
        reminder_arg0s = list(dict.fromkeys(
            row.entries[cmap[schema.Fields.Arg0]].formatted
            for row in status_response.rows
        ))
        if not reminder_arg0s:
            return common.list_rows("vt.reminders", {}, request, VALUES)

        # Query 2: get all attributes for these reminders
        attr_map, raw_assn_pks = common.get_fields_for_items(
            self.client, "", reminder_arg0s)

        # Queries 3 & 4: find today's day plan and which reminders are in it
        plan_pk = common.get_todays_day_plan(self.client)
        entry_refs = common.get_day_plan_entry_refs(self.client, plan_pk)

        entries = {}
        for arg0 in reminder_arg0s:
            fields = attr_map.get(arg0, {})
            field_assn_pks = dict(raw_assn_pks.get(arg0, {}))

            # Remap internal relation name to display column name
            if "TargetDate" in field_assn_pks:
                field_assn_pks["Target Date"] = field_assn_pks.pop("TargetDate")

            # Encode the day-plan assn pk so IncrementEntry can delete it
            if arg0 in entry_refs:
                field_assn_pks["Active"] = [[entry_refs[arg0], f"@{{vt.reminders {arg0}}}"]]
                active = ["Today"]
            else:
                active = ["Later"]

            task_val = fields.get("Task", [""])[0]
            check_val = fields.get("Check", [""])[0]
            description = check_val if check_val else task_val

            status_raw = fields.get("Status", [""])[0]
            pk = common.encode_pk(arg0, field_assn_pks)
            entries[pk] = {
                "_pk": [pk],
                "Active": active,
                "Priority": fields.get("Priority", []),
                "Target Date": fields.get("TargetDate", []),
                "Description": [description] if description else [],
                "Status": [common.colorize_status(status_raw)] if status_raw else [],
            }

        return common.list_rows("vt.reminders", entries, request, VALUES)

    def IncrementEntry(self, request, context):
        arg0, pk_map = common.decode_pk(request.pk)

        if request.column == "Active":
            current = "Today" if "Active" in pk_map else "Later"
            values = VALUES["Active"]
            next_value = values[(values.index(current) + request.amount) % len(values)]
            if next_value == current:
                return jql_pb2.IncrementEntryResponse()
            plan_pk = common.get_todays_day_plan(self.client)
            if plan_pk is None:
                raise ValueError("No active day plan found for today")
            if next_value == "Today":
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((".Entry", f"tasks {plan_pk}", arg0)),
                    fields={
                        schema.Fields.Relation: ".Entry",
                        schema.Fields.Arg0: f"tasks {plan_pk}",
                        schema.Fields.Arg1: f"@{{vt.reminders {arg0}}}",
                    },
                    insert_only=True,
                ))
            else:
                assn_pk, _ = pk_map["Active"][0]
                self.client.DeleteRow(jql_pb2.DeleteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                ))

        elif request.column in ("Status", "Priority"):
            if request.column in pk_map:
                assn_pk, current = pk_map[request.column][0]
                values = VALUES[request.column]
                current_index = values.index(current) if current in values else 0
                next_value = values[(current_index + request.amount) % len(values)]
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                    fields={schema.Fields.Arg1: next_value},
                    update_only=True,
                ))
            else:
                value = VALUES[request.column][0]
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((f".{request.column}", arg0, "0000")),
                    fields={
                        schema.Fields.Relation: f".{request.column}",
                        schema.Fields.Arg0: arg0,
                        schema.Fields.Arg1: value,
                    },
                    insert_only=True,
                ))

        elif request.column == "Target Date":
            today = datetime.date.today().strftime(_DATE_FORMAT)
            new_value = f"@{{{schema.Tables.Dates} {today}}}"
            if "Target Date" in pk_map:
                assn_pk, current = pk_map["Target Date"][0]
                if common.is_date_foreign(current):
                    new_value = common.increment_date_foreign(current, request.amount)
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=assn_pk,
                    fields={schema.Fields.Arg1: new_value},
                    update_only=True,
                ))
            else:
                self.client.WriteRow(jql_pb2.WriteRowRequest(
                    table=schema.Tables.Assertions,
                    pk=str((".TargetDate", arg0, "0000")),
                    fields={
                        schema.Fields.Relation: ".TargetDate",
                        schema.Fields.Arg0: arg0,
                        schema.Fields.Arg1: new_value,
                    },
                    insert_only=True,
                ))

        else:
            raise ValueError("Cannot increment column", request.column)

        return jql_pb2.IncrementEntryResponse()
