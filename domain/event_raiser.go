package domain

type EventRaiser struct {
	events []Event
}

func (er *EventRaiser) Raise(e Event) {
	er.events = append(er.events, e)
}

func (er *EventRaiser) Events() []Event {
	return er.events
}

func (er *EventRaiser) Clear() {
	er.events = nil
}

func (er *EventRaiser) Drain() []Event {
	events := er.events
	er.events = nil
	return events
}
