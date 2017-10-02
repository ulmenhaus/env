import click
import datetime
import httplib2

from apiclient import discovery

from googul import creds


def _list_calendars(service):
    entries = []
    page_token = None
    while True:
        calendar_list = service.calendarList().list(
            pageToken=page_token).execute()
        entries.extend(calendar_list['items'])
        page_token = calendar_list.get('nextPageToken')
        if not page_token:
            break
    return entries


def ls():
    """
    List all reminders
    """
    credentials = creds.get_oauth_creds()
    http = credentials.authorize(httplib2.Http())
    service = discovery.build('calendar', 'v3', http=http)

    now = datetime.datetime.utcnow().isoformat() + 'Z'  # 'Z' indicates UTC time
    print('Getting the upcoming 10 events')
    print(_list_calendars(service))
    eventsResult = service.events().list(
        calendarId='primary',
        timeMin=now,
        maxResults=10,
        singleEvents=True,
        orderBy='startTime').execute()
    events = eventsResult.get('items', [])

    if not events:
        print('No upcoming events found.')
    for event in events:
        start = event['start'].get('dateTime', event['start'].get('date'))
        print(start, event['summary'])
