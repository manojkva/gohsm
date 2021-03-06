/*
Package hsm provides the framework for Hierarchical State Machine implementations.

Related Documents:
    - Introduction to Hierarchical State Machines: https://barrgroup.com/Embedded-Systems/How-To/Introduction-Hierarchical-State-Machines
    - Yet Another Hierarchical State Machine: https://accu.org/index.php/journals/252
    - State Diagram: https://en.wikipedia.org/wiki/State_diagram
    - gohsm Object Model: go_state_machine_framework.png

Included in this framework are the following components:

  - StateMachine:
    Machine that controls the event processing

  - State:
    Interface that must be implemented by all States in the StateMachine
    Composed of an InternalState interface that is implemented by BaseState
    and an ExternalState interface whose methods must be implemented by each state in the state machine

  - Transition:
    Interface that is implemented by each of the different types of transitions:

      - ExternalTransition:
        Transition from current state to a different state.  On execution the following takes place:
          1. OnExit is called on the current state and all parent states up to the parent state that owns
             the new state (or the parent state is nil)
          2. action() associated with the the transition is called
          3. OnEnter() is called on the new state which may call OnEnter() for a sub-state.  The final
             new current state is returned by the OnEnter() call

      - InternalTransition:
        Transition within the current state.  On execution the following takes place:
          1. action() associated with the the transition is called

      - EndTransition:
        Transition from current state that terminates the state machine.  On execution the following takes place:
          1. OnExit is called on the current state and all parent states until there are no more parent states
          2. action() associated with the the transition is called

  - Event:
    An event represents something that has happened (login, logout, newCall, networkChange, etc.) that might drive
    a change in the state machine
*/
package hsm

import (
	"context"
	"go.uber.org/zap"
)

// StateMachine manages event processing as implemented by each State
type StateMachine struct {
	currentState State
	logger       *zap.Logger
}

// NewStateMachine constructor
func NewStateMachine(logger *zap.Logger, startState State, startEvent Event) *StateMachine {
	sm := &StateMachine{
		currentState: startState,
		logger:       logger,
	}

	// This will ensure we are in the proper state starting from the beginning.
	sm.initialize(startEvent)
	return sm
}

// CurrentState getter
func (sm *StateMachine) CurrentState() State {
	return sm.currentState
}

func (sm *StateMachine) initialize(startEvent Event) {
	sm.currentState = sm.currentState.OnEnter(startEvent)
	sm.logger.Debug("state machine initialized",
		zap.String("starting_state", sm.currentState.Name()),
	)
}

// HandleEvent executes the event handler for the current state or parent state if found.
// If no event handler is found then the event is dropped
func (sm *StateMachine) HandleEvent(e Event) bool {
	// Find an event handler (if none found then skip the event)
	sm.logger.Debug("HandleEvent: checking if event " + e.ID() + " is handled in state " + sm.currentState.Name())
	transition := sm.currentState.EventHandler(e)
	parentState := sm.currentState.ParentState()
	for transition == nil {
		if parentState == nil {
			sm.logger.Debug("HandleEvent: state " + sm.currentState.Name() + " has a nil parentState ... fuck!!!")
			// Skip event handling
			return false
		}

		sm.logger.Debug("HandleEvent: checking if event " + e.ID() + " is handled in state " + parentState.Name())
		transition = parentState.EventHandler(e)
		parentState = parentState.ParentState()
	}

	// Handle the event and update the current state
	sm.currentState = transition.Execute(sm.logger, sm.currentState)

	return true
}

// Run starts the StateMachine and processes incoming events until the StateMachine
// terminates (new currentState is nil after processing a transition) or the "done" event is received
func (sm *StateMachine) Run(ctx context.Context, events <-chan Event) {
	go func() {
		for {
			select {
			case e, ok := <-events:
				if !ok {
					return
				}

				sm.logger.Debug("handling event",
					zap.String("event_id", e.ID()),
					zap.String("current_state", sm.currentState.Name()),
				)

				handled := sm.HandleEvent(e)
				if !handled {
					sm.logger.Debug("event not handled",
						zap.String("event_id", e.ID()),
					)
					continue
				}

				if sm.currentState == nil {
					sm.logger.Debug("current state nil, terminating run loop")
					return
				}

				sm.logger.Debug("handled event",
					zap.String("current_state", sm.currentState.Name()),
				)
			case <-ctx.Done():
				sm.logger.Debug("received done on svc")
				return
			}
		}
	}()
}
