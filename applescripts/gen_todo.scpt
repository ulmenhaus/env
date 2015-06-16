tell application "Google Chrome" to get URL of active tab of front window
set active_url to text of result
tell application "Google Chrome" to get title of active tab of front window
set active_title to text of result

tell application "Mail"
    activate
    set the_message to make new outgoing message with properties {visible:true, subject:active_title, content:active_url}
    tell the_message to make new to recipient with properties {name:"Ryan Abrams", address:"todo.rdabrams@gmail.com"}
end tell

tell application "System Events"
    tell pop up button "From:" of window 1 of application process "Mail"
        click
        tell menu item "Ryan Abrams – rdabrams@gmail.com" of menu 1 to click
    end tell
    tell text field 2 of window 1 of application process "Mail" to click
end tell

# HACK I don't know how to select the Subject field
tell application "System Events" to keystroke tab
tell application "System Events" to keystroke tab
