package test

import (
	"context"
	"github.com/fabric8-services/fabric8-auth/app"
	"github.com/fabric8-services/fabric8-auth/authentication/account/repository"
	testservice "github.com/fabric8-services/fabric8-auth/test/generated/application/service"
	"github.com/fabric8-services/fabric8-auth/wit"
	goauuid "github.com/goadesign/goa/uuid"
	"github.com/gojuno/minimock"
)

func NewWITMock(t minimock.Tester, inviteeID, spaceName string) *testservice.WITServiceMock {
	witServiceMock := testservice.NewWITServiceMock(t)
	witServiceMock.GetSpaceFunc = func(p context.Context, spaceID string) (r *wit.Space, e error) {
		ownerID, e := goauuid.FromString(inviteeID)
		if e != nil {
			return nil, e
		}
		return &wit.Space{OwnerID: ownerID, Name: spaceName}, nil
	}
	witServiceMock.CreateUserFunc = func(p context.Context, identity *repository.Identity, identityID string) (r error) {
		return nil
	}

	witServiceMock.UpdateUserFunc = func(p context.Context, updatePayload *app.UpdateUsersPayload, identityID string) (r error) {
		return nil
	}

	return witServiceMock
}
