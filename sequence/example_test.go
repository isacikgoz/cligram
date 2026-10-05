package sequence_test

import (
	"fmt"

	"github.com/isacikgoz/cligram"
	"github.com/isacikgoz/cligram/sequence"
)

func Example() {
	d := sequence.New()
	d.Participant("alice", "Alice", sequence.Actor())
	d.Participant("api", "API")
	d.Message("alice", "api", "GET /orders", sequence.Activate())
	d.Block("alt", "found")
	d.Message("api", "alice", "200 orders", sequence.Reply(), sequence.Deactivate())
	d.Section("else", "missing")
	d.Message("api", "alice", "404", sequence.Reply(), sequence.WithHead(sequence.Cross))
	d.End()

	l := d.Layout(sequence.Fit(80, 24))
	fmt.Println(l.Render(cligram.Plain))
	// Output:
	// ┏━━━━━━━┓       ╭─────╮
	// ┃ Alice ┃       │ API │
	// ┗━━━┯━━━┛       ╰──┬──╯
	//     │ GET /orders  │
	//     ├─────────────►┃
	//   ╭╌│╌ alt found ╌╌┃╌╮
	//   ╎ │  200 orders  ┃ ╎
	//   ╎ │◄┄┄┄┄┄┄┄┄┄┄┄┄┄┨ ╎
	//   ╎╌│╌ else missing ╌╎
	//   ╎ │     404      │ ╎
	//   ╎ │╳┄┄┄┄┄┄┄┄┄┄┄┄┄┤ ╎
	//   ╰╌│╌╌╌╌╌╌╌╌╌╌╌╌╌╌│╌╯
	//     │              │
}
