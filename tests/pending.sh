# Pending mark for model-level test cases (DESIGN.md, Tests). Source it from a
# test script, write the case as a function, and call it through `pending`:
#   case_x() { ...; }
#   pending case_x
# The case is skipped until its task removes the prefix; RUN_PENDING=1 runs it.
pending() {
  if [ "${RUN_PENDING:-}" = 1 ]; then
    "$@"
  else
    echo "skip $1"
  fi
}
