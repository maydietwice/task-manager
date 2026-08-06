package clientinterceptor

import (
	"context"
	"fmt"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type contextKey string

const ChatIDKey contextKey = "chat_id"

func JWTClientInterceptor(secret string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		jwtToken := jwt.New(jwt.SigningMethodHS256)

		chatID, ok := ctx.Value(ChatIDKey).(int64)
		if !ok {
			return fmt.Errorf("context %v expected type: int, got: %T", ChatIDKey, ctx.Value(ChatIDKey))
		}

		jwtToken.Claims = jwt.MapClaims{
			"owner_id": strconv.Itoa(int(chatID)),
		}

		token, err := jwtToken.SignedString([]byte(secret))
		if err != nil {
			return err
		}

		md := metadata.Pairs("authorization", "Bearer "+token)

		ctx = metadata.NewOutgoingContext(ctx, md)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
