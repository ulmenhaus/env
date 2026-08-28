"""
Markdown export for timedb vt.relatives identity view.
"""

from timedb import schema, client_utils


def _pluralize(word):
    if word.endswith("es"):
        return word
    if word.endswith("y") and len(word) > 2 and word[-2] not in "aeou":
        return word[:-1] + "ies"
    if word.endswith("is"):
        return word[:-2] + "es"
    if word.endswith("s") or word.endswith("h"):
        return word + "es"
    return word + "s"


def _get_grouped_relation(request):
    for grouping in request.group_by.groupings:
        if grouping.field == "Relation":
            return grouping.selected
    return None


def _is_bulleted_list(entries):
    """
    Determine display style for a multiton attribute's entries.

    Returns True for a bulleted list, False for named sections.

    Dead giveaways (checked first):
    - Any entry with "  * " sub-bullets → bulleted list
    - Any entry with "  - " sub-hyphens → named sections

    Fallback heuristic: any multi-line or long entry → named sections,
    otherwise bulleted list.
    """
    for entry in entries:
        if "  * " in entry:
            return True
        if "  - " in entry:
            return False
    for entry in entries:
        if "\n" in entry:
            return False
        if len(entry) > 100:
            return False
    return True


def _format_as_bullets(entries):
    return "\n".join(f"* {entry}" for entry in entries)


def _format_as_named_sections(entries):
    parts = []
    for entry in entries:
        title, _, body = entry.partition("\n")
        title = title.strip()
        if not title.startswith("### "):
            title = f"### {title}"
        parts.append(title)
        if body:
            parts.append(body)
    return "\n".join(parts)


def _generate_table_markdown(dbms, request, relation):
    request.limit = 0
    response = dbms.ListRows(request)
    primary, cmap = client_utils.list_rows_meta(response)

    cols = [
        col for col in response.columns
        if not col.name.startswith("_")
    ]
    headers = [col.display_value or col.name for col in cols]
    col_indices = [cmap[col.name] for col in cols]

    def cell(entry):
        return entry.display_value or entry.formatted

    rows = [
        [cell(row.entries[i]) for i in col_indices]
        for row in response.rows
    ]

    lines = []
    lines.append("| " + " | ".join(headers) + " |")
    lines.append("| " + " | ".join("---" for _ in headers) + " |")
    for row in rows:
        lines.append("| " + " | ".join(row) + " |")

    item_full_pk = client_utils.selected_target(request)
    _, pk = item_full_pk.split(" ", 1)
    markdown = f"# {pk} — {relation}\n\n"
    markdown += "\n".join(lines) + "\n"
    return markdown


def _generate_identity_markdown(dbms, item_full_pk):
    fields, _ = client_utils.get_fields_for_items(dbms, "", [item_full_pk])
    item_fields = fields[item_full_pk]

    singletons = {}
    multitons = {}

    for attr, values in item_fields.items():
        if not values:
            continue
        if len(values) == 1 and not values[0].startswith("### "):
            singletons[attr] = values[0]
        else:
            multitons[attr] = values

    _, pk = item_full_pk.split(" ", 1)
    markdown = f"# {pk}\n\n"

    if singletons:
        headers = sorted(singletons.keys())
        markdown += "| " + " | ".join(f"**{h}**" for h in headers) + " |\n"
        markdown += "| " + " | ".join("---" for _ in headers) + " |\n"
        markdown += "| " + " | ".join(str(singletons[h]) for h in headers) + " |\n"

    for attr in sorted(multitons.keys()):
        values = multitons[attr]
        plural = _pluralize(attr)
        markdown += f"\n## {plural} ({len(values)})\n"
        if _is_bulleted_list(values):
            markdown += _format_as_bullets(values)
        else:
            markdown += _format_as_named_sections(values)
        markdown += "\n"

    return markdown


def generate_markdown(iface):
    """
    Generate markdown from a macro handle whose view is vt.relatives
    grouped by Relation=w/ Identity.
    """
    dbms = iface.get_dbms()
    request = iface.get_request()

    if request.table != schema.Tables.Relatives:
        raise ValueError(
            f"Only works on vt.relatives table, got: {request.table!r}")

    relation = _get_grouped_relation(request)
    if relation is None:
        raise ValueError("View must be grouped by Relation")

    item_full_pk = client_utils.selected_target(request)
    if item_full_pk is None:
        raise ValueError("Could not determine target item from request")

    if relation == schema.Values.RelationIdentity:
        return _generate_identity_markdown(dbms, item_full_pk)

    return _generate_table_markdown(dbms, request, relation)
