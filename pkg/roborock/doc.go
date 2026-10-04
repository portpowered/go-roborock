// Package roborock provides a stateless cloud client and explicit typed device sessions.
// Account credentials belong to requests; each device session owns its connection
// and closes when its opening context ends, the transport fails, or Close is called.
package roborock
