import collections
import json

from timedb import pks, schema
from timedb.virtual_gateway import common
from timedb import client_utils

from jql import jql_pb2, jql_pb2_grpc


def _parse_days_until(days_until):
    if not days_until:
        return float('inf')
    if days_until.startswith('+'):
        return int(days_until[1:])
    return int(days_until)


class PracticesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client

    def ListRows(self, request, context):
        actionable = self.query_practices()
        return common.list_rows('vt.practices', actionable, request)

    def query_practices(self, hide_active=True):
        feeds_resp = self._query_feeds()
        return self._query_actionable_children(feeds_resp, hide_active)
        
    def _query_feeds(self):
        nouns_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(column=schema.Fields.Feed,
                                   negated=True,
                                   equal_match=jql_pb2.EqualMatch(value="")),
                ], ),
            ],
        )
        return self.client.ListRows(nouns_request)

    def _query_actionable_children(self, feeds_resp, hide_active):
        primary = client_utils.get_primary(feeds_resp)
        feeds = {row.entries[primary].formatted for row in feeds_resp.rows}
        feed_attrs, _ = client_utils.get_fields_for_items(self.client,
                                                    schema.Tables.Nouns, feeds)
        cmap = {c.name: i for i, c in enumerate(feeds_resp.columns)}
        action_map = {
            schema.Values.StatusExploring: "Explore",
            schema.Values.StatusPlanning: "Plan",
            schema.Values.StatusImplementing: "Implement",
            schema.Values.StatusHabitual: "Implement",
            schema.Values.StatusSatisfied: "Implement",
            schema.Values.StatusRevisit: "Implement",
        }
        towards_map = {
            schema.Values.StatusExploring: "something new",
            schema.Values.StatusPlanning: "something new",
            schema.Values.StatusImplementing: "something new",
            schema.Values.StatusHabitual: schema.Values.TowardsSomethingRegular,
            schema.Values.StatusRevisit: "something special",
            schema.Values.StatusSatisfied: "something vintage",
        }
        nouns_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Parent,
                        in_match=jql_pb2.InMatch(values=sorted(feeds))),
                    jql_pb2.Filter(
                        column=schema.Fields.Status,
                        in_match=jql_pb2.InMatch(values=sorted(action_map))),
                ], ),
            ],
        )
        nouns = self.client.ListRows(nouns_request)
        primary = client_utils.get_primary(nouns)
        noun_pks = [row.entries[primary].formatted for row in nouns.rows]
        children = {}
        # TODO active_actions is now available on TimingInfo so we don't have
        # to query active_tasks separately
        active_tasks = self.client.ListRows(
            jql_pb2.ListRowsRequest(
                table=schema.Tables.Tasks,
                conditions=[
                    jql_pb2.Condition(requires=[
                        jql_pb2.Filter(
                            column=schema.Fields.Status,
                            in_match=jql_pb2.InMatch(values=[
                                schema.Values.StatusPending,
                                schema.Values.StatusPlanned,
                                schema.Values.StatusActive,
                            ]),
                        ),
                    ]),
                ],
            ))
        tasks_cmap = {c.name: i for i, c in enumerate(active_tasks.columns)}
        active_pairs = {
            (task.entries[tasks_cmap[schema.Fields.Action]].formatted,
             task.entries[tasks_cmap[schema.Fields.Direct]].formatted)
            for task in active_tasks.rows
        }
        row_attrs, _ = client_utils.get_fields_for_items(self.client,
                                                   schema.Tables.Nouns,
                                                   noun_pks)
        pk2row = {}
        for row in nouns.rows:
            pk = row.entries[primary].formatted
            pk2row[pk] = row

        for row in nouns.rows:
            parent = row.entries[cmap[schema.Fields.Parent]].formatted
            local_action_map = dict(action_map)
            if "Feed.Action" in feed_attrs[parent]:
                local_action_map[
                    schema.Values.
                    StatusImplementing] = feed_attrs[parent]["Feed.Action"][0]
                local_action_map[schema.Values.StatusHabitual] = feed_attrs[
                    parent]["Feed.Action"][0]
                local_action_map[schema.Values.StatusSatisfied] = feed_attrs[
                    parent]["Feed.Action"][0]
                local_action_map[schema.Values.StatusRevisit] = feed_attrs[
                    parent]["Feed.Action"][0]
            towards = towards_map[row.entries[cmap[
                schema.Fields.Status]].formatted]
            action = local_action_map[row.entries[cmap[
                schema.Fields.Status]].formatted]
            domain = feed_attrs[parent].get("Domain", [''])[0]
            skillset = feed_attrs[parent].get("Feed.Skillset", [''])[0]
            if domain == "" and parent in pk2row:
                parent_row = pk2row[parent]
                grandparent = parent_row.entries[cmap[schema.Fields.Parent]].formatted
                domain = feed_attrs[grandparent].get("Domain", [''])[0]
                skillset = feed_attrs[grandparent].get("Feed.Skillset", [''])[0]
            source = f"@{{nouns {parent}}}"
            genre = feed_attrs[parent].get("Feed.Genre", [''])[0]
            motivation = feed_attrs[parent].get("Feed.Motivation", [''])[0]
            direct = row.entries[primary].formatted
            if 'yes' in feed_attrs[parent].get(
                    "Feed.StripContext", []) or row.entries[
                        cmap[schema.Fields.
                             Modifier]].formatted == common.ALIAS_MODIFIER:
                direct = row.entries[cmap[schema.Fields.Description]].formatted
            if (action, direct) in active_pairs and hide_active:
                # Don't show practices that already have active tasks
                continue
            practice = f"{action} {direct}"
            children[practice] = {
                "_pk": [practice],
                "Action": [action],
                "Direct": [direct],
                "Source": [source],
                "Domain": [domain],
                "Skillset": [skillset],
                "Genre": [genre],
                "Motivation": [motivation],
                "Towards": [towards],
            }
        for feed in feeds_resp.rows:
            if feed.entries[cmap[schema.Fields.Feed]].formatted != 'manual':
                continue
            actions = ["Ideate", "Triage", "Appraise"]
            direct = feed.entries[primary].formatted
            parent = feed.entries[cmap[schema.Fields.Parent]].formatted
            domain = feed_attrs[direct].get("Domain", [''])[0]
            skillset = feed_attrs[direct].get("Feed.Skillset", [''])[0]
            genre = feed_attrs[direct].get("Feed.Genre", [''])[0]
            for action in actions:
                if (action, direct) in active_pairs:
                    continue
                practice = f"{action} {direct}"
                children[practice] = {
                    "_pk": [practice],
                    "Action": [action],
                    "Direct": [direct],
                    "Source": [f"@{{nouns {parent}}}"],
                    "Domain": [domain],
                    "Skillset": [skillset],
                    "Motivation": ['Investment'],
                    "Genre": [genre],
                    "Towards": ['something regular'],
                }
        # First pass: set Days Since and Days Until from each entry's own timing info
        all_noun_pks = set(row['Direct'][0] for row in children.values())
        noun2info = common.get_timing_info(self.client, all_noun_pks)
        for child in children.values():
            direct = child['Direct'][0]
            if direct not in noun2info:
                continue
            info = noun2info[direct]
            child['Days Since'] = [info.days_since]
            child['Days Until'] = [info.days_until]
        # Second pass: for Practice entries, tighten Days Until to also reflect the
        # most urgent child belonging to that skillset — a Practice is due as soon
        # as any of its constituent items are due
        self._apply_practice_skillset_days_until(children)
        return children

    def _apply_practice_skillset_days_until(self, children):
        skillset_min_days_until = {}
        for child in children.values():
            skillset_raw = child.get("Skillset", [""])[0]
            if not skillset_raw:
                continue
            if client_utils.is_foreign(skillset_raw):
                _, skillset_pk = client_utils.parse_foreign(skillset_raw)
            else:
                skillset_pk = skillset_raw
            if not skillset_pk:
                continue
            days_until = child.get("Days Until", [""])[0]
            if not days_until:
                continue
            current = skillset_min_days_until.get(skillset_pk)
            if current is None or _parse_days_until(days_until) < _parse_days_until(current):
                skillset_min_days_until[skillset_pk] = days_until
        for child in children.values():
            if child.get("Action", [""])[0] != "Practice":
                continue
            direct = child.get("Direct", [""])[0]
            if direct not in skillset_min_days_until:
                continue
            child_days_until = child.get("Days Until", [""])[0]
            skillset_days_until = skillset_min_days_until[direct]
            if not child_days_until or _parse_days_until(skillset_days_until) < _parse_days_until(child_days_until):
                child["Days Until"] = [skillset_days_until]

    def GetRow(self, request, context):
        return common.get_row(
            self.ListRows(jql_pb2.ListRowsRequest(), context), request.pk)
