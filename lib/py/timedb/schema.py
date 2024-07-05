class Tables(object):
    Actions = "actions"
    Assertions = 'assertions'
    Contexts = "contexts"
    Nouns = 'nouns'
    Tasks = 'tasks'


class Fields(object):
    Action = "Action"
    Arg0 = "Arg0"
    Arg1 = "Arg1"
    Code = "Code"
    Context = "Context"
    Coordinal = "Coordinal"
    Description = "Description"
    Direct = "Direct"
    Disambiguator = "Disambiguator"
    Domain = "Domain"
    Feed = "Feed"
    Identifier = "_Identifier"
    Indirect = "Indirect"
    Link = "Link"
    Modifier = "A Modifier"
    Motivation = "Motivation"
    NounRelation = "Relation"
    Order = "Order"
    Parameters = "Parameters"
    ParamStart = "Param~Start"
    Parent = "Parent"
    PrimaryGoal = "Primary Goal"
    Relation = "A Relation"
    Source = "Source"
    Status = 'Status'
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


def active_statuses():
    return [
        Values.StatusActive, Values.StatusExploring, Values.StatusHabitual, Values.StatusIdea,
        Values.StatusImplementing, Values.StatusPlanned, Values.StatusPlanning
    ]
