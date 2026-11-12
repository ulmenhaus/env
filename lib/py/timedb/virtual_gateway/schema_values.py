"""Utilities for working with schematized values in timedb.

A profile is a noun with .Dimension attributes, each of which references a
schema noun (e.g. @{nouns Mood}). A schema noun has one or more .ValueSet
attributes describing the values an attribute of that schema can take.
"""

from timedb import client_utils, schema

from jql import jql_pb2


def get_dimensions(client, profile_full_pks):
    """Returns a map from each profile's full pk ("nouns <pk>") to the
    list of dimensions (schema references) defined on that profile."""
    profile_fields, _ = client_utils.get_fields_for_items(
        client, "", profile_full_pks, fields=['Dimension'])
    return {
        profile: list(fields['Dimension'])
        for profile, fields in profile_fields.items()
    }


def get_values_for_schemas(client, schema_refs):
    """Returns a map from each schema reference (e.g. @{nouns Mood}) to the
    list of values that an attribute of that schema can take."""
    schema_full_pks = [
        client_utils.strip_foreign(ref) for ref in set(schema_refs)
        if client_utils.is_foreign(ref) and ref
        and client_utils.parse_foreign(ref)[0] == schema.Tables.Nouns
    ]
    schema_attrs, _ = client_utils.get_fields_for_items(client,
                                                        "",
                                                        schema_full_pks,
                                                        fields=['ValueSet'])
    schema2values = {}
    for schema_name, fields in schema_attrs.items():
        schema_key = f"@{{{schema_name}}}"
        schema2values[schema_key] = []
        for value_def in fields['ValueSet']:
            # NOTE if we have multiple @{nouns } style defs then getting them serially
            # is a little inefficient, but is worth the tradeoff for cleaner code
            schema2values[schema_key].extend(
                get_schema_values(client, value_def))
    return schema2values


def get_schema_values(client, schema_value_def):
    """Returns the values described by a single .ValueSet definition"""
    table, pk = client_utils.parse_foreign(schema_value_def)
    if table == "ratings":
        total = int(pk)
        return [f"@{{ratings {i} {total}}}" for i in range(total + 1)]
    if table == schema.Tables.Nouns:
        nouns_request = jql_pb2.ListRowsRequest(
            table=schema.Tables.Nouns,
            conditions=[
                jql_pb2.Condition(requires=[
                    jql_pb2.Filter(
                        column=schema.Fields.Parent,
                        equal_match=jql_pb2.EqualMatch(value=pk),
                    ),
                ], )
            ],
        )
        nouns_response = client.ListRows(nouns_request)
        primary, _ = client_utils.list_rows_meta(nouns_response)
        noun_references = [
            f"@{{{schema.Tables.Nouns} {noun.entries[primary].formatted}}}"
            for noun in nouns_response.rows
        ]
        # Merge the results with the members of the noun
        #
        # NOTE once we implement include_children=True for client_utils.get_fields_for_items
        # we won't need to do this merging manually. To do this though we may need
        # to audit direct children to ensure they have a parent relation set.
        member_fields, _ = client_utils.get_fields_for_items_by_pk(
            client, schema.Tables.Nouns, [pk], fields=['Member'])
        noun_references.extend(member_fields[pk]['Member'])
        return noun_references
    # e.g. ints which are free-form
    return []
