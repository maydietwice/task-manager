package middleware

import (
	"context"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func JWTInterceptor(secret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		if info.FullMethod == "/task.TaskService/Register" {
			return handler(ctx, req)
		}
		md, ok := metadata.FromIncomingContext(ctx)

		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		tokens := md.Get("authorization")

		if len(tokens) < 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		tSplitted := strings.Split(tokens[0], " ")

		if len(tSplitted) < 2 {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		if tSplitted[0] != "Bearer" {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		jwtToken, err := jwt.ParseWithClaims(tSplitted[1], jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{"HS256"}))

		if err != nil {
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}

		claims, ok := jwtToken.Claims.(jwt.MapClaims)

		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		ownerId, ok := claims["owner_id"].(string)

		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		ctx = context.WithValue(ctx, "owner_id", ownerId)

		return handler(ctx, req)

	}
}
