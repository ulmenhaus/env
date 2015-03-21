# functionals
More Higher-order functions for python callables

## installation
### With virtual environment (recommended)
Create a virtual environment with python3 and install nose and flake8 (I should put dependencies in a requirements.txt file)

## testing
Run "run_tests.sh" (I should consider using tox or something that can test in multiple environments)

## Potential Additions

### Generator Pipeline
- Useful when a we want a stream data by passing it through a chain of generators; yielding in the first generator should cause the second to yield should cause the third to yield &c
- Consider allowing for a cyclic connection of generators
- example: streaming a file through multiple per-line transforms

### Dynamic Programming
- automatic detection of the dependencies between sub-calls and in-order traversal of the dependency graph