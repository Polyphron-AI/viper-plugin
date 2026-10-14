package kit

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestFail checks Fail is InvalidArgument with its message, which Viper
// shows as a 400 with that text instead of a generic plugin failure.
func TestFail(t *testing.T) {
	got, ok := status.FromError(Fail("Give a name to greet."))

	if !ok || got.Code() != codes.InvalidArgument || got.Message() != "Give a name to greet." {
		t.Fatalf(
			"Fail gave %v",
			got,
		)
	}
}
