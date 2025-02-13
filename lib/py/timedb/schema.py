class Tables(object):
    Actions = "actions"
    Assertions = 'assertions'
    Attributes = "vt.attributes"
    Contexts = "contexts"
    Nouns = 'nouns'
    Relatives = "vt.relatives"
    Tasks = 'tasks'


class Fields(object):
    Action = "Action"
    Arg0 = "Arg0"
    Arg1 = "Arg1"
    AttributeRelation = "Relation"
    Code = "Code"
    Context = "Context"
    Coordinal = "Coordinal"
    Description = "Description"
    Direct = "Direct"
    Disambiguator = "Disambiguator"
    Domain = "Domain"
    Feed = "Feed"
    Genre = "Genre"
    Identifier = "_Identifier"
    Indirect = "Indirect"
    Item = "-> Item"
    Link = "Link"
    Modifier = "A Modifier"
    Motivation = "Motivation"
    NounRelation = "Relation"
    Order = "Order"
    ParamStart = "Param~Start"
    Parameters = "Parameters"
    Parent = "Parent"
    PrimaryGoal = "Primary Goal"
    Relation = "A Relation"
    RelativeRelation = "Relation"
    Source = "Source"
    Status = 'Status'
    Subset = 'Subset'
    Towards = "Towards"
    UDescription = "_Description"


class Values(object):
    StatusActive = "Active"
    StatusExploring = "Exploring"
    StatusHabitual = 'Habitual'
    StatusIdea = 'Idea'
    StatusImplementing = "Implementing"
    StatusPending = "Pending"
    StatusPlanned = "Planned"
    StatusPlanning = "Planning"
    StatusRevisit = 'Revisit'
    StatusRevisit = 'Revisit'
    StatusSatisfied = 'Satisfied'

    ModifierPlanFor = 'Plan for'

    RelationIdentity = 'w/ Identity'

def active_statuses():
    return [
        Values.StatusActive, Values.StatusExploring, Values.StatusHabitual, Values.StatusIdea,
        Values.StatusImplementing, Values.StatusPlanned, Values.StatusPlanning
    ]

def primary_for_table(table):
    if table == Tables.Nouns:
        return Fields.Identifier
    elif table == Tables.Tasks:
        return Fields.UDescription
    return ""
