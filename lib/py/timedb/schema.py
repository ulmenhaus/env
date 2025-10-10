class Tables(object):
    Actions = "actions"
    Assertions = 'assertions'
    Attributes = "vt.attributes"
    Contexts = "contexts"
    Files = 'files'
    Nouns = 'nouns'
    Relatives = "vt.relatives"
    Tasks = 'tasks'
    Ratings = 'ratings'


class Fields(object):
    Action = "Action"
    Arg0 = "Arg0"
    Arg1 = "Arg1"
    AttributeRelation = "Relation"
    Class = "Class"
    Code = "Code"
    Context = "Context"
    Coordinal = "Coordinal"
    Description = "Description"
    Direct = "Direct"
    Disambiguator = "Disambiguator"
    DisplayName = "Display Name"
    Domain = "Domain"
    Feed = "Feed"
    Genre = "Genre"
    Habitual = "Habitual"
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
    Profile = "Profile"
    Relation = "A Relation"
    RelativeRelation = "Relation"
    Skillset = "Skillset"
    Source = "Source"
    Status = 'Status'
    Subset = 'Subset'
    Towards = "Towards"
    UDescription = "_Description"
    UPK = "_pk"


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

    TowardsSomethingRegular = "something regular"

    ActionTrack = "Track"
    DirectLifeEvents = "life events & practices"


def active_statuses():
    return [
        Values.StatusActive, Values.StatusExploring, Values.StatusHabitual,
        Values.StatusIdea, Values.StatusImplementing, Values.StatusPlanned,
        Values.StatusPlanning
    ]


def primary_for_table(table):
    if table == Tables.Nouns:
        return Fields.Identifier
    elif table == Tables.Tasks:
        return Fields.UDescription
    return ""


class ProjectManagementValues(object):
    ActionWorkOnProject = "Work"
    ActionExecuteProjectPlan = "Execute"
    ActionFocusOnArea = "Focus"

    @staticmethod
    def is_goal_action(action):
        return action in ["Extend", "Improve", "Sustain"]

    @staticmethod
    def is_phase_action(action):
        """
        Phases of projects are the subsets of their work that are scoped to
        particular goal cycles. They tie together workstreams and goals in
        three ways:

        **Work tasks**: specify subsets of workstreams from a project plan
        **Execute tasks**: imply that a whole project plan is in scope
        **Focus tasks**: denote focus areas with goals and workstreams but don't show as projects
        """
        return action in ["Work", "Execute", "Focus"]


class SpecialClassesForRelatives(object):
    FeedClass = "Feed"


def relation_from_parameter_schema(ps):
    for part in ps.split(" "):
        if part and part[0].isupper():
            return part
    return "Unknown"

def indirect_indicates_habit(indirect):
    return indirect in ["regularity", "incrementality", "breakdown", "habituality", "continuity"]
