#!/bin/sh
# The run the demo shows: events, a second or less apart, as a CI script
# would write them.
e() { echo "$1"; sleep "${2:-0.45}"; }
sleep 0.5
e "push done" 0.3
e "push -> lint" 0.1
e "lint active"
e "lint done" 0.1
e "lint -> compile" 0.1
e "compile active" 0.6
e "compile done" 0.1
e "compile -> unit" 0.1
e "unit active" 0.6
e "unit done" 0.1
e "unit -> stage" 0.1
e "stage active" 0.6
e "stage done" 0.1
e "stage -> smoke" 0.1
e "smoke active" 0.7
e "smoke done" 0.1
e "smoke -> prod" 0.1
e "prod done" 0.3
