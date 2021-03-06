package service_test

import (
	"context"
	"testing"

	"github.com/fabric8-services/fabric8-auth/app"
	"github.com/fabric8-services/fabric8-auth/application/service"
	"github.com/fabric8-services/fabric8-auth/application/service/factory"
	account "github.com/fabric8-services/fabric8-auth/authentication/account/repository"
	"github.com/fabric8-services/fabric8-auth/authorization"
	"github.com/fabric8-services/fabric8-auth/authorization/invitation"
	invitationrepo "github.com/fabric8-services/fabric8-auth/authorization/invitation/repository"
	"github.com/fabric8-services/fabric8-auth/errors"
	"github.com/fabric8-services/fabric8-auth/gormapplication"
	"github.com/fabric8-services/fabric8-auth/gormtestsupport"
	"github.com/fabric8-services/fabric8-auth/notification"
	"github.com/fabric8-services/fabric8-auth/rest"
	"github.com/fabric8-services/fabric8-auth/test"
	testservice "github.com/fabric8-services/fabric8-auth/test/generated/application/service"

	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	spaceName = "my-test-space"
	success   = "https://success"
	failure   = "https://failure"
)

type invitationServiceBlackBoxTest struct {
	gormtestsupport.DBTestSuite
	invitationRepo          invitationrepo.InvitationRepository
	identityRepo            account.IdentityRepository
	orgService              service.OrganizationService
	notificationServiceMock *testservice.NotificationServiceMock
	witServiceMock          *testservice.WITServiceMock
}

func TestRunInvitationServiceBlackBoxTest(t *testing.T) {
	suite.Run(t, &invitationServiceBlackBoxTest{DBTestSuite: gormtestsupport.NewDBTestSuite()})
}

func (s *invitationServiceBlackBoxTest) SetupTest() {
	s.DBTestSuite.SetupTest()
	s.invitationRepo = invitationrepo.NewInvitationRepository(s.DB)
	s.identityRepo = account.NewIdentityRepository(s.DB)
	s.orgService = s.Application.OrganizationService()
	s.notificationServiceMock = testservice.NewNotificationServiceMock(s.T())
	s.witServiceMock = testservice.NewWITServiceMock(s.T())
	s.Application = gormapplication.NewGormDB(s.DB, s.Configuration, s.Wrappers, factory.WithWITService(s.witServiceMock), factory.WithNotificationService(s.notificationServiceMock))
}

func (s *invitationServiceBlackBoxTest) TestIssueInvitation() {
	acceptInvitationEndpoint := "/api/invitations/accept/"

	s.T().Run("should issue invitation by identity id", func(t *testing.T) {
		// given
		g := s.NewTestGraph(t)

		// Create a test user - this will be the team admin
		teamAdmin := g.CreateUser()

		// Create a team
		team := g.CreateTeam()

		// Create an admin role and assign it to the team admin
		r := g.CreateRole(g.LoadResourceType(authorization.IdentityResourceTypeTeam))
		r.AddScope(authorization.ManageTeamMembersScope)

		team.AssignRole(teamAdmin.Identity(), r.Role())

		// Create another test user - we will invite this one to join the team
		invitee := g.CreateUser()
		id := invitee.IdentityID()

		invitations := []invitation.Invitation{
			{
				IdentityID:        &id,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		var messages []notification.Message
		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			messages = msgs
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, teamAdmin.IdentityID().String(), spaceName)

		// when
		err := s.Application.InvitationService().Issue(s.Ctx, teamAdmin.IdentityID(), team.TeamID().String(), invitations)

		// then
		require.NoError(t, err, "Error creating invitations")
		require.Equal(t, uint64(1), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Len(t, messages, 1)
		require.Equal(t, id.String(), messages[0].TargetID)
		require.Contains(t, messages[0].Custom["acceptURL"], acceptInvitationEndpoint)
		require.Equal(t, uint64(1), s.witServiceMock.GetSpaceCounter)

		invs, err := s.invitationRepo.ListForIdentity(s.Ctx, team.TeamID())

		require.NoError(t, err, "Error listing invitations")
		require.Equal(t, 1, len(invs))
		require.True(t, invs[0].Member)
		require.Equal(t, success, invs[0].SuccessRedirectURL)
		require.Equal(t, failure, invs[0].FailureRedirectURL)
	})

	s.T().Run("should fail to issue invitation for invalid id", func(t *testing.T) {
		// given
		// Create a test user - this will be the inviter
		identity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestIssuingUser")
		require.Nil(t, err, "Could not create identity")

		// Create another test user - we will invite this one to join the nonexistent "thing"
		otherIdentity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestInviteeUser")
		require.Nil(t, err, "Could not create other identity")

		invitations := []invitation.Invitation{
			{
				IdentityID:        &otherIdentity.ID,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, uuid.NewV4().String(), spaceName)

		// when
		err = s.Application.InvitationService().Issue(s.Ctx, identity.ID, uuid.NewV4().String(), invitations)

		// then
		require.Error(t, err)
		require.Equal(t, uint64(0), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Equal(t, uint64(0), s.witServiceMock.GetSpaceCounter)

		// when
		err = s.Application.InvitationService().Issue(s.Ctx, identity.ID, "foo", invitations)

		// then
		require.Error(t, err)
		require.Equal(t, uint64(0), s.notificationServiceMock.SendMessagesAsyncCounter)
	})

	s.T().Run("should issue invitation for resource", func(t *testing.T) {
		// given
		g := s.NewTestGraph(t)

		// Create a test user - this will be the inviter
		inviter := g.CreateUser()

		// Create another test user - we will invite this one to accept a role for the resource
		invitee := g.CreateUser()
		inviteeID := invitee.IdentityID()

		space := g.CreateSpace()
		space.AddAdmin(inviter)

		// Create an invitation
		invitations := []invitation.Invitation{
			{
				IdentityID:        &inviteeID,
				Roles:             []string{"admin"},
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		var messages []notification.Message
		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			messages = msgs
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, inviter.IdentityID().String(), spaceName)

		// when - issue the invitation
		err := s.Application.InvitationService().Issue(s.Ctx, inviter.IdentityID(), space.SpaceID(), invitations)

		// then
		require.NoError(t, err)
		require.Equal(t, uint64(1), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Len(t, messages, 1)
		require.Equal(t, inviteeID.String(), messages[0].TargetID)
		require.Contains(t, messages[0].Custom["acceptURL"], acceptInvitationEndpoint)
		require.Equal(t, uint64(1), s.witServiceMock.GetSpaceCounter)

		//List the invitations for our resource
		invs, err := s.invitationRepo.ListForResource(s.Ctx, space.SpaceID())
		require.NoError(t, err, "Error listing invitations")

		// There should be 1 invitation only
		require.Equal(t, 1, len(invs))
		require.False(t, invs[0].Member)
		require.Equal(t, invitee.IdentityID(), invs[0].IdentityID)

		// there should be success and failure redirect url
		require.Equal(t, success, invs[0].SuccessRedirectURL)
		require.Equal(t, failure, invs[0].FailureRedirectURL)

		//List the roles for our invitation
		roles, err := s.invitationRepo.ListRoles(s.Ctx, invs[0].InvitationID)
		require.NoError(t, err, "Error listing roles")

		// There should be 1 role only
		require.Equal(t, 1, len(roles))
		require.Equal(t, "admin", roles[0].Name)
	})

	s.T().Run("should fail to issue member invitation for resource", func(t *testing.T) {
		// given
		// Create a test user - this will be the inviter
		identity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestIssuingUser")
		require.NoError(t, err)

		// Create another test user - we will invite this one to accept a role for the resource
		otherIdentity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestInviteeUser")
		require.NoError(t, err)

		// Create a new resource type
		resourceType, err := test.CreateTestResourceType(s.Ctx, s.DB, "invitation.test/foo")
		require.NoError(t, err)

		// Create the manage members scope for the new resource type (we will use the same scope as for organizations)
		scope, err := test.CreateTestScope(s.Ctx, s.DB, *resourceType, authorization.ManageOrganizationMembersScope)
		require.NoError(t, err)

		// Create an admin role for the resource type
		role, err := test.CreateTestRole(s.Ctx, s.DB, *resourceType, "admin")
		require.NoError(t, err)

		// Assign the scope to our role
		_, err = test.CreateTestRoleScope(s.Ctx, s.DB, *scope, *role)
		require.NoError(t, err)

		// Create a resource
		resource, err := test.CreateTestResource(s.Ctx, s.DB, *resourceType, "InvitationTestResourceFoo", nil)
		require.NoError(t, err)

		// Assign the owner role to our user for the resource
		test.CreateTestIdentityRoleForIdentity(s.Ctx, s.DB, identity, *resource, *role)
		require.NoError(t, err)

		// Create an invitation
		invitations := []invitation.Invitation{
			{
				IdentityID:        &otherIdentity.ID,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, uuid.NewV4().String(), spaceName)

		// when - issue the invitation, which should fail because the new resource can't have members
		err = s.Application.InvitationService().Issue(s.Ctx, identity.ID, resource.ResourceID, invitations)

		// then
		require.Error(t, err)
		require.Equal(t, uint64(0), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Equal(t, uint64(0), s.witServiceMock.GetSpaceCounter)
	})

	s.T().Run("should fail to issue unprivileged invitation for resource", func(t *testing.T) {
		// given
		// Create a test user - this will be the inviter
		identity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestIssuingUser")
		require.NoError(t, err)

		// Create another test user - we will invite this one to accept a role for the resource
		otherIdentity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestInviteeUser")
		require.NoError(t, err)

		// Create a new resource type
		resourceType, err := test.CreateTestResourceType(s.Ctx, s.DB, "invitation.test/bar")
		require.NoError(t, err)

		// Create an admin role for the resource type
		_, err = test.CreateTestRole(s.Ctx, s.DB, *resourceType, "admin")
		require.NoError(t, err)

		// Create a resource
		resource, err := test.CreateTestResource(s.Ctx, s.DB, *resourceType, "InvitationTestResourceFoo", nil)
		require.NoError(t, err)

		// Create an invitation
		invitations := []invitation.Invitation{
			{
				IdentityID:        &otherIdentity.ID,
				Roles:             []string{"admin"},
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, uuid.NewV4().String(), spaceName)

		// when - issue the invitation, which should fail because the inviter has insufficient privileges to issue an invitation
		err = s.Application.InvitationService().Issue(s.Ctx, identity.ID, resource.ResourceID, invitations)

		// then
		require.Error(t, err)
		require.Equal(t, uint64(0), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Equal(t, uint64(0), s.witServiceMock.GetSpaceCounter)
	})

	s.T().Run("should fail to issue invitation for non owner", func(t *testing.T) {
		// given
		// Create a test user - this will be the owner
		identity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestIssuingUser")
		require.Nil(t, err, "Could not create identity")

		// Create an organization
		orgId, err := s.orgService.CreateOrganization(s.Ctx, identity.ID, "Test Organization "+uuid.NewV4().String())
		require.Nil(t, err, "Could not create organization")

		// Create another test user - we will attempt to have this user invite themselves to the organization
		otherIdentity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestInviteeUser")
		require.Nil(t, err, "Could not create other identity")

		invitations := []invitation.Invitation{
			{
				IdentityID:        &otherIdentity.ID,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, uuid.NewV4().String(), spaceName)

		// when
		err = s.Application.InvitationService().Issue(s.Ctx, otherIdentity.ID, orgId.String(), invitations)

		// then
		require.Error(t, err)
		require.Equal(t, uint64(0), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Equal(t, uint64(0), s.witServiceMock.GetSpaceCounter)
	})

	s.T().Run("should fail to issue invitation for unknown user", func(t *testing.T) {
		// given
		// Create a test user - this will be the organization owner
		identity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestIssuingUser")
		require.Nil(t, err, "Could not create identity")

		// Create an organization
		orgId, err := s.orgService.CreateOrganization(s.Ctx, identity.ID, "Test Organization "+uuid.NewV4().String())
		require.Nil(t, err, "Could not create organization")

		invalidIdentityID := uuid.NewV4()

		invitations := []invitation.Invitation{
			{
				IdentityID:        &invalidIdentityID,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, uuid.NewV4().String(), spaceName)

		// when - this should fail because we specified an unknown identity ID
		err = s.Application.InvitationService().Issue(s.Ctx, identity.ID, orgId.String(), invitations)

		// then
		require.Error(t, err)
		require.Equal(t, uint64(0), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Equal(t, uint64(0), s.witServiceMock.GetSpaceCounter)
	})

	s.T().Run("should fail to issue invitation for non user", func(t *testing.T) {
		// given
		// Create a test user - this will be the organization owner
		identity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestIssuingUser")
		require.Nil(t, err, "Could not create identity")

		// Create an organization, we're going to do something crazy and invite the organization to join itself
		orgId, err := s.orgService.CreateOrganization(s.Ctx, identity.ID, "Test Organization "+uuid.NewV4().String())
		require.Nil(t, err, "Could not create organization")

		invitations := []invitation.Invitation{
			{
				IdentityID:        orgId,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, uuid.NewV4().String(), spaceName)

		// when - This should fail because we specified a non-user identity in the invitation
		err = s.Application.InvitationService().Issue(s.Ctx, identity.ID, orgId.String(), invitations)

		// then
		require.Error(t, err)
		require.Equal(t, uint64(0), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Equal(t, uint64(0), s.witServiceMock.GetSpaceCounter)
	})

	s.T().Run("should fail to issue invitation for non membership identity", func(t *testing.T) {
		// given
		// Create a test user - this will be the inviter, and the identity to which the other identity will be invited
		identity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestIssuingUser")
		require.Nil(t, err, "Could not create identity")

		// Create an invitee
		otherIdentity, err := test.CreateTestIdentityAndUserWithDefaultProviderType(s.DB, "invitationServiceBlackBoxTest-TestInviteeUser")
		require.Nil(t, err, "Could not create other identity")

		invitations := []invitation.Invitation{
			{
				IdentityID:        &otherIdentity.ID,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, uuid.NewV4().String(), spaceName)

		// when - invite the user to "join" the other user as a member, this should fail
		err = s.Application.InvitationService().Issue(s.Ctx, identity.ID, identity.ID.String(), invitations)

		// then
		require.Error(t, err)
		require.Equal(t, uint64(0), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Equal(t, uint64(0), s.witServiceMock.GetSpaceCounter)
	})

	s.T().Run("should issue multiple invitations", func(t *testing.T) {
		// given
		team := s.Graph.CreateTeam()
		teamAdmin := s.Graph.CreateUser()

		r := s.Graph.CreateRole(s.Graph.LoadResourceType(authorization.IdentityResourceTypeTeam))
		r.AddScope(authorization.ManageTeamMembersScope)

		team.AssignRole(teamAdmin.Identity(), r.Role())

		invitee1 := s.Graph.CreateUser()
		invitee1ID := invitee1.IdentityID()

		invitee2 := s.Graph.CreateUser()
		invitee2ID := invitee2.IdentityID()

		invitations := []invitation.Invitation{
			{
				IdentityID:        &invitee1ID,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
			{
				IdentityID:        &invitee2ID,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		var messages []notification.Message
		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			messages = msgs
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, teamAdmin.IdentityID().String(), spaceName)

		// when
		err := s.Application.InvitationService().Issue(s.Ctx, teamAdmin.IdentityID(), team.TeamID().String(), invitations)

		// then
		require.NoError(t, err, "Error creating invitations")
		require.Equal(t, uint64(1), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Len(t, messages, 2)
		require.Equal(t, invitee1ID.String(), messages[0].TargetID)
		require.Contains(t, messages[0].Custom["acceptURL"], acceptInvitationEndpoint)
		require.Equal(t, invitee2ID.String(), messages[1].TargetID)
		require.Contains(t, messages[1].Custom["acceptURL"], acceptInvitationEndpoint)
		require.Equal(t, uint64(1), s.witServiceMock.GetSpaceCounter)

		invs, err := s.invitationRepo.ListForIdentity(s.Ctx, team.TeamID())
		require.NoError(t, err, "Error listing invitations")
		require.Equal(t, 2, len(invs))

		found := false

		for _, inv := range invs {
			if inv.IdentityID == invitee1.IdentityID() {
				found = true
				require.True(t, inv.Member)
				require.Equal(t, invitee1.IdentityID(), inv.IdentityID)
				require.Equal(t, team.TeamID(), *inv.InviteTo)
				require.Equal(t, success, inv.SuccessRedirectURL)
				require.Equal(t, failure, inv.FailureRedirectURL)
			}
		}

		require.True(t, found, "First invitee not found in invitations")

		found = false
		for _, inv := range invs {
			if inv.IdentityID == invitee2.IdentityID() {
				found = true
				require.True(t, inv.Member)
				require.Equal(t, success, inv.SuccessRedirectURL)
				require.Equal(t, failure, inv.FailureRedirectURL)
			}
		}
		require.True(t, found, "Second invitee not found in invitations")
	})

	s.T().Run("should issue TestIssueInvitationByIdentityIDForRole", func(t *testing.T) {
		// given
		team := s.Graph.CreateTeam()
		teamAdmin := s.Graph.CreateUser()
		user := s.Graph.CreateUser()
		r := s.Graph.CreateRole(s.Graph.LoadResourceType(authorization.IdentityResourceTypeTeam))
		r.AddScope(authorization.ManageTeamMembersScope)

		team.AssignRole(teamAdmin.Identity(), r.Role())

		id := user.IdentityID()

		invitations := []invitation.Invitation{
			{
				IdentityID:        &id,
				Roles:             []string{r.Role().Name},
				Member:            false,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		var messages []notification.Message
		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			messages = msgs
			return nil, nil
		}

		// when
		err := s.Application.InvitationService().Issue(s.Ctx, teamAdmin.IdentityID(), team.TeamID().String(), invitations)

		// then
		require.NoError(t, err, "Error creating invitations")
		require.Equal(t, uint64(1), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Len(t, messages, 1)
		require.Equal(t, id.String(), messages[0].TargetID)

		invs, err := s.invitationRepo.ListForIdentity(s.Ctx, team.TeamID())
		require.NoError(t, err, "Error listing invitations")
		require.Len(t, invs, 1)
		require.False(t, invs[0].Member)

		roles, err := s.invitationRepo.ListRoles(s.Ctx, invs[0].InvitationID)
		require.NoError(t, err, "could not list roles")
		require.Len(t, roles, 1)
		require.Equal(t, r.Role().Name, roles[0].Name)

		require.Equal(t, success, invs[0].SuccessRedirectURL)
		require.Equal(t, failure, invs[0].FailureRedirectURL)
	})

	s.T().Run("should issue invitation for team member", func(t *testing.T) {
		// given
		team := s.Graph.CreateTeam()
		teamAdmin := s.Graph.CreateUser()
		user := s.Graph.CreateUser()
		r := s.Graph.CreateRole(s.Graph.LoadResourceType(authorization.IdentityResourceTypeTeam))
		r.AddScope(authorization.ManageTeamMembersScope)

		team.AssignRole(teamAdmin.Identity(), r.Role())

		id := user.IdentityID()

		invitations := []invitation.Invitation{
			{
				IdentityID:        &id,
				Roles:             nil,
				Member:            true,
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		var messages []notification.Message
		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			messages = msgs
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, teamAdmin.IdentityID().String(), spaceName)

		// when
		err := s.Application.InvitationService().Issue(s.Ctx, teamAdmin.IdentityID(), team.TeamID().String(), invitations)

		//then
		require.NoError(t, err)
		require.Equal(t, uint64(1), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Len(t, messages, 1)
		require.Equal(t, id.String(), messages[0].TargetID)
		require.Contains(t, messages[0].Custom["acceptURL"], acceptInvitationEndpoint)
		require.Equal(t, uint64(1), s.witServiceMock.GetSpaceCounter)

		invs, err := s.invitationRepo.ListForIdentity(s.Ctx, team.TeamID())
		require.NoError(t, err)

		require.Len(t, invs, 1)
		require.Equal(t, user.IdentityID(), invs[0].IdentityID)
		require.True(t, invs[0].Member)
		require.Equal(t, success, invs[0].SuccessRedirectURL)
		require.Equal(t, failure, invs[0].FailureRedirectURL)
	})

	s.T().Run("should issue invitation for space", func(t *testing.T) {
		// given
		space := s.Graph.CreateSpace()
		spaceAdmin := s.Graph.CreateUser()
		space.AddAdmin(spaceAdmin)

		invitee := s.Graph.CreateUser()
		id := invitee.IdentityID()

		r := s.Graph.CreateRole("foo", s.Graph.LoadResourceType(authorization.ResourceTypeSpace))

		invitations := []invitation.Invitation{
			{
				IdentityID:        &id,
				Roles:             []string{r.Role().Name},
				RedirectOnSuccess: success,
				RedirectOnFailure: failure,
			},
		}

		var messages []notification.Message
		*s.notificationServiceMock = *testservice.NewNotificationServiceMock(t)
		s.notificationServiceMock.SendMessagesAsyncFunc = func(p context.Context, msgs []notification.Message, p2 ...rest.HTTPClientOption) (r chan error, r1 error) {
			messages = msgs
			return nil, nil
		}

		*s.witServiceMock = *test.NewWITMock(t, spaceAdmin.IdentityID().String(), spaceName)

		// when
		err := s.Application.InvitationService().Issue(s.Ctx, spaceAdmin.IdentityID(), space.SpaceID(), invitations)

		// then
		require.NoError(t, err)
		require.Equal(t, uint64(1), s.notificationServiceMock.SendMessagesAsyncCounter)
		require.Len(t, messages, 1)
		require.Equal(t, id.String(), messages[0].TargetID)
		require.Contains(t, messages[0].Custom["acceptURL"], acceptInvitationEndpoint)
		require.Equal(t, uint64(1), s.witServiceMock.GetSpaceCounter)

		invs, err := s.invitationRepo.ListForResource(s.Ctx, space.SpaceID())

		require.NoError(t, err)
		require.Len(t, invs, 1)
		require.Equal(t, invitee.IdentityID(), invs[0].IdentityID)
		require.False(t, invs[0].Member)
		require.Equal(t, success, invs[0].SuccessRedirectURL)
		require.Equal(t, failure, invs[0].FailureRedirectURL)
	})
}

func (s *invitationServiceBlackBoxTest) TestAcceptInvitation() {
	s.T().Run("should accept team membership invitation", func(t *testing.T) {
		// given
		team := s.Graph.CreateTeam()
		user := s.Graph.CreateUser()
		inv := s.Graph.CreateInvitation(team, user, true, redirectURL())

		// when
		resourceID, redirectPath, err := s.Application.InvitationService().Accept(s.Ctx, inv.Invitation().AcceptCode)

		// then
		require.NoError(t, err)
		require.Equal(t, team.ResourceID(), resourceID)
		require.Equal(t, success, redirectPath)

		assocs, err := s.Application.Identities().FindIdentityMemberships(s.Ctx, user.IdentityID(), nil)
		require.NoError(t, err)

		require.Len(t, assocs, 1)
		require.Equal(t, team.TeamID(), *assocs[0].IdentityID)
		require.True(t, assocs[0].Member)
		require.Empty(t, assocs[0].Roles)

		// verify then invitation is deleted after accepting it
		_, err = s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)
		require.Error(t, err)
		require.IsType(t, errors.NotFoundError{}, err)

	})

	s.T().Run("should accept team role invitation", func(t *testing.T) {
		// given
		team := s.Graph.CreateTeam()
		user := s.Graph.CreateUser()
		teamRole := s.Graph.CreateRole(s.Graph.LoadResourceType(authorization.IdentityResourceTypeTeam))
		inv := s.Graph.CreateInvitation(team, user, false, teamRole, redirectURL())

		// when
		resourceID, redirectPath, err := s.Application.InvitationService().Accept(s.Ctx, inv.Invitation().AcceptCode)

		// then
		require.NoError(t, err)
		require.Equal(t, team.ResourceID(), resourceID)
		require.Equal(t, success, redirectPath)

		assocs, err := s.Application.IdentityRoleRepository().FindIdentityRolesForIdentity(s.Ctx, user.IdentityID(), nil)
		require.NoError(t, err)

		require.Len(t, assocs, 1)
		require.Equal(t, team.TeamID(), *assocs[0].IdentityID)
		require.False(t, assocs[0].Member)
		require.Len(t, assocs[0].Roles, 1)
		require.Equal(t, teamRole.Role().Name, assocs[0].Roles[0])

		// verify then invitation is deleted after accepting it
		_, err = s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)
		require.Error(t, err)
		require.IsType(t, errors.NotFoundError{}, err)
	})

	s.T().Run("should accept space invitation", func(t *testing.T) {
		// given
		space := s.Graph.CreateSpace()
		user := s.Graph.CreateUser()
		spaceRole := s.Graph.CreateRole(s.Graph.LoadResourceType(authorization.ResourceTypeSpace))
		inv := s.Graph.CreateInvitation(space, user, spaceRole, redirectURL())

		// when
		resourceID, redirectPath, err := s.Application.InvitationService().Accept(s.Ctx, inv.Invitation().AcceptCode)

		// then
		require.NoError(t, err)
		require.Equal(t, space.SpaceID(), resourceID)
		require.Equal(t, success, redirectPath)

		roles, err := s.Application.IdentityRoleRepository().FindIdentityRolesForIdentity(s.Ctx, user.IdentityID(), nil)
		require.NoError(t, err)

		require.Len(t, roles, 1)
		require.Equal(t, space.SpaceID(), roles[0].ResourceID)
		require.False(t, roles[0].Member)
		require.Len(t, roles[0].Roles, 1)
		require.Equal(t, spaceRole.Role().Name, roles[0].Roles[0])

		// Test that the accept code cannot be used again
		// when
		resourceID, redirectPath, err = s.Application.InvitationService().Accept(s.Ctx, inv.Invitation().AcceptCode)

		//then
		require.Error(t, err)
		require.Empty(t, resourceID)
		require.Empty(t, redirectPath)
	})

	s.T().Run("should fail to accept invitation for unknown accept code", func(t *testing.T) {
		// given
		space := s.Graph.CreateSpace()
		user := s.Graph.CreateUser()
		spaceRole := s.Graph.CreateRole(s.Graph.LoadResourceType(authorization.ResourceTypeSpace))
		s.Graph.CreateInvitation(space, user, spaceRole)

		// when
		resourceID, redirectPath, err := s.Application.InvitationService().Accept(s.Ctx, uuid.NewV4())

		//then
		require.Error(t, err)
		require.Empty(t, resourceID)
		require.Empty(t, redirectPath)
	})
}

func (s *invitationServiceBlackBoxTest) TestRescindInvitation() {
	s.T().Run("should rescind invitation for invitation id", func(t *testing.T) {
		// given
		// Create a test user - this will be the team admin
		teamAdmin := s.Graph.CreateUser()

		// Create a team
		team := s.Graph.CreateTeam()

		// Create another test user - we will invite this one to join the team
		invitee := s.Graph.CreateUser()

		r := s.Graph.CreateRole(s.Graph.LoadResourceType(authorization.IdentityResourceTypeTeam))
		r.AddScope(authorization.ManageTeamMembersScope)
		team.AssignRole(teamAdmin.Identity(), r.Role())

		inv := s.Graph.CreateInvitation(team, invitee, true)

		// when
		err := s.Application.InvitationService().Rescind(s.Ctx, teamAdmin.IdentityID(), inv.Invitation().InvitationID)

		// then
		require.NoError(t, err, "Error rescinding invitation")

		_, err = s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)

		require.Error(t, err)
		require.IsType(t, errors.NotFoundError{}, err)

	})
	s.T().Run("should fail to rescind  unprivileged invitation for invitation id", func(t *testing.T) {
		// given
		// Create a test user - this will be the team admin
		teamAdmin := s.Graph.CreateUser()

		// Create a team
		team := s.Graph.CreateTeam()

		// Create another test user - we will invite this one to join the team
		invitee := s.Graph.CreateUser()

		inv := s.Graph.CreateInvitation(team, invitee, true)

		// when
		err := s.Application.InvitationService().Rescind(s.Ctx, teamAdmin.IdentityID(), inv.Invitation().InvitationID)

		// then
		require.Error(t, err, "Error rescinding invitation")
		require.IsType(t, errors.ForbiddenError{}, err)

		_, err = s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)
		require.NoError(t, err)
	})
	s.T().Run("should fail to rescind  invitation for invalid invitation id", func(t *testing.T) {
		// given
		// Create a test user - this will be the team admin
		teamAdmin := s.Graph.CreateUser()

		// Create a team
		team := s.Graph.CreateTeam()

		// Create another test user - we will invite this one to join the team
		invitee := s.Graph.CreateUser()

		inv := s.Graph.CreateInvitation(team, invitee, true)

		// when
		err := s.Application.InvitationService().Rescind(s.Ctx, teamAdmin.IdentityID(), uuid.NewV4())

		// then
		require.Error(t, err, "Error rescinding invitation")
		require.IsType(t, errors.NotFoundError{}, err)

		_, err = s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)
		require.NoError(t, err)
	})

	s.T().Run("should rescind invitation for resource", func(t *testing.T) {
		// given
		// Create a test user - this will be the team admin
		teamAdmin := s.Graph.CreateUser()

		// Create resource space
		space := s.Graph.CreateSpace()
		space.AddAdmin(teamAdmin)

		// Create another test user - we will invite this one to join space
		invitee := s.Graph.CreateUser()

		inv := s.Graph.CreateInvitation(space, invitee, true)

		// when
		err := s.Application.InvitationService().Rescind(s.Ctx, teamAdmin.IdentityID(), inv.Invitation().InvitationID)

		// then
		require.NoError(t, err, "Error rescinding invitation")

		_, err = s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)
		require.Error(t, err)
		require.IsType(t, errors.NotFoundError{}, err)

	})

	s.T().Run("should fail to rescind unauthorised invitation for resource", func(t *testing.T) {
		// given
		// Create a test user - this will be the team admin
		teamAdmin := s.Graph.CreateUser()

		// Create resource space
		space := s.Graph.CreateSpace()

		// Create another test user - we will invite this one to join space
		invitee := s.Graph.CreateUser()

		inv := s.Graph.CreateInvitation(space, invitee, true)

		// when
		err := s.Application.InvitationService().Rescind(s.Ctx, teamAdmin.IdentityID(), inv.Invitation().InvitationID)

		// then
		require.Error(t, err, "Error rescinding invitation")
		require.IsType(t, errors.ForbiddenError{}, err)

		_, err = s.Application.InvitationRepository().Load(s.Ctx, inv.Invitation().InvitationID)
		require.NoError(t, err)
	})
}

func (s *invitationServiceBlackBoxTest) TestAcceptBasicResourceInvitationUpdatesPrivilegeCache() {
	// Create a new resource type with scope "foo"
	rt := s.Graph.CreateResourceType()
	rt.AddScope("foo")

	// Create a role for the new resource type, with scope "foo"
	role := s.Graph.CreateRole(rt)
	role.AddScope("foo")

	// Create a new resource
	res := s.Graph.CreateResource(rt)

	// Create a user
	user := s.Graph.CreateUser()

	// Invite the user to accept the role for the resource
	inv := s.Graph.CreateInvitation(res, user, role, false)

	// At this point in time, the user should have no scopes for the resource
	privs, err := s.Application.PrivilegeCacheService().CachedPrivileges(s.Ctx, user.IdentityID(), res.ResourceID())
	require.NoError(s.T(), err)
	require.Len(s.T(), privs.ScopesAsArray(), 0)

	// Accept the invitation
	_, _, err = s.Application.InvitationService().Accept(s.Ctx, inv.Invitation().AcceptCode)
	require.NoError(s.T(), err)

	// The user should now have the "foo" scope
	privs, err = s.Application.PrivilegeCacheService().CachedPrivileges(s.Ctx, user.IdentityID(), res.ResourceID())
	require.NoError(s.T(), err)
	require.Len(s.T(), privs.ScopesAsArray(), 1)
	require.Contains(s.T(), privs.ScopesAsArray(), "foo")
}

func redirectURL() *app.RedirectURL {
	success := success
	failure := failure

	return &app.RedirectURL{&failure, &success}
}
