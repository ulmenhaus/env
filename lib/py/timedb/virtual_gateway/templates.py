"""vt.templates lets the user fill out a template noun.

A template is a noun with the assertion .Class = @{nouns Template}. It has a
.Schema attribute referencing a profile (see vt.profiles) whose .Dimension
attributes each reference a schema with a value-set, and a .Text attribute
which is a python format string. The table has one column per dimension
which the user populates (as enums of the schema's values) and a final Text
column which is the template's text interpolated with those values.

Within the text a dimension is referenced by the pk of its schema noun,
e.g. a dimension of @{nouns Mood} is referenced as {Mood}. The name between
the braces is taken literally (so pks like "[schemas] Topic" work) and {{ / }}
produce literal braces. Dimensions that haven't been populated yet are left
as-is in the rendered text.
"""

import collections
import re

from timedb import client_utils, schema
from timedb.virtual_gateway import common, schema_values

from jql import jql_pb2, jql_pb2_grpc

TEMPLATE_CLASS = "Template"
TABLE_NAME = "vt.templates"
TARGET_COLUMN = "-> Item"
TEXT_COLUMN = "Text"


class TemplatesBackend(jql_pb2_grpc.JQLServicer):

    def __init__(self, client):
        super().__init__()
        self.client = client
        # The populated values for each template, keyed by the template's
        # full pk and then by dimension. This state is kept only in memory
        # for the lifetime of the gateway.
        self.state = collections.defaultdict(dict)

    def ListRows(self, request, context):
        targets = client_utils.selected_targets(request, column=TARGET_COLUMN)
        if not targets:
            return common.possible_targets(
                self.client,
                request,
                TABLE_NAME,
                candidates=client_utils.nouns_with_class(
                    self.client, TEMPLATE_CLASS),
            )
        rows, schema2values = self._query_templates(targets)
        return common.list_rows(TABLE_NAME,
                                rows,
                                request,
                                values=schema2values,
                                client=self.client)

    def _query_templates(self, targets):
        template_fields, _ = client_utils.get_fields_for_items(
            self.client, "", targets, fields=['Schema', 'Text'])
        template_to_profile = {
            template: client_utils.strip_foreign(fields['Schema'][0])
            for template, fields in template_fields.items()
            if fields['Schema'] and client_utils.is_foreign(fields['Schema'][0])
        }
        profile_to_dimensions = schema_values.get_dimensions(
            self.client, list(set(template_to_profile.values())))
        all_dimensions = set().union(*profile_to_dimensions.values())
        schema2values = schema_values.get_values_for_schemas(
            self.client, all_dimensions)
        rows = {}
        for template, fields in template_fields.items():
            dimensions = profile_to_dimensions.get(
                template_to_profile.get(template), [])
            populated = self.state[template]
            row = {
                "_pk": [template],
                TARGET_COLUMN: [template],
                TEXT_COLUMN: [
                    render("\n".join(fields['Text']), dimensions, populated)
                ],
            }
            for dimension in dimensions:
                value = populated.get(dimension, "")
                row[dimension] = [value] if value else []
            rows[template] = row
        return rows, schema2values

    def GetRow(self, request, context):
        rows, _ = self._query_templates([request.pk])
        return common.return_row(TABLE_NAME, rows[request.pk])

    def WriteRow(self, request, context):
        populated = self.state[request.pk]
        for field, value in request.fields.items():
            if field in (TARGET_COLUMN, TEXT_COLUMN, "_pk"):
                continue
            populated[field] = value
        return jql_pb2.WriteRowResponse()


def _interpolation_key(dimension):
    if client_utils.is_foreign(dimension) and dimension:
        table, pk = client_utils.parse_foreign(dimension)
        if table == schema.Tables.Nouns:
            return pk
    return dimension


# Matches an escaped brace ({{ or }}) or a {placeholder}. The placeholder is
# taken literally (unlike str.format) since dimension names are noun pks that
# commonly contain characters like [, ., : and ! which str.format would
# interpret as indexing, attribute access, format specs, or conversions.
_PLACEHOLDER = re.compile(r"\{\{|\}\}|\{([^{}]+)\}")


def render(text, dimensions, populated):
    """Interpolates text with the populated values of each dimension.
    Placeholders for dimensions that aren't populated are left as-is."""
    values = {}
    for dimension in dimensions:
        value = populated.get(dimension, "")
        if value:
            values[_interpolation_key(dimension)] = client_utils.present_attrs(
                [value])

    def substitute(match):
        token = match.group(0)
        if token == "{{":
            return "{"
        if token == "}}":
            return "}"
        return values.get(match.group(1), token)

    return _PLACEHOLDER.sub(substitute, text)
