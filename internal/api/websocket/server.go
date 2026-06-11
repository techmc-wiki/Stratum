package websocket

// Hub reserves the future real-time event boundary. The MVP intentionally does
// not add a WebSocket dependency or unauthenticated event transport.
type Hub struct{}

// TODO: implement authenticated session status and log event streaming.
