package routes

import (
	"github.com/championswimmer/api.midpoint.place/src/config"
	"github.com/championswimmer/api.midpoint.place/src/controllers"
	"github.com/championswimmer/api.midpoint.place/src/db/models"
	"github.com/championswimmer/api.midpoint.place/src/dto"
	"github.com/championswimmer/api.midpoint.place/src/security"
	"github.com/championswimmer/api.midpoint.place/src/security/ratelimit"
	"github.com/championswimmer/api.midpoint.place/src/server/parsers"
	"github.com/championswimmer/api.midpoint.place/src/server/validators"
	"github.com/championswimmer/api.midpoint.place/src/services"
	"github.com/championswimmer/api.midpoint.place/src/utils/applogger"
	"github.com/gofiber/fiber/v2"
)

var groupsController *controllers.GroupsController
var groupUsersController *controllers.GroupUsersController
var groupPlacesController *controllers.GroupPlacesController
var placesSearchService *services.PlaceSearchService

func GroupsRoute() func(router fiber.Router) {
	groupsController = controllers.CreateGroupsController()
	groupUsersController = controllers.CreateGroupUsersController()
	groupPlacesController = controllers.CreateGroupPlacesController()
	placesSearchService = services.NewPlaceSearchService()

	return func(router fiber.Router) {
		router.Get("/", security.MandatoryJwtAuthMiddleware, listPublicGroups)
		router.Post("/", security.MandatoryJwtAuthMiddleware, ratelimit.GroupCreateRateLimiter(), createGroup)
		router.Get("/:groupIdOrCode", security.MandatoryJwtAuthMiddleware, getGroup)
		router.Patch("/:groupIdOrCode", security.MandatoryJwtAuthMiddleware, updateGroup)
		router.Put("/:groupIdOrCode/join", security.MandatoryJwtAuthMiddleware, joinGroup)
		router.Delete("/:groupIdOrCode/join", security.MandatoryJwtAuthMiddleware, leaveGroup)
	}
}

// @Summary List public groups
// @Description Get a list of all public groups, ordered by creation date (newest first), limited to 100 results
// @Tags groups
// @ID list-public-groups
// @Produce json
// @Param self query string false "Filter groups - 'creator' for groups created by user, 'member' for groups user belongs to" Enums(creator,member)
// @Success 200 {array} dto.GroupResponse "List of public groups"
// @Failure 500 {object} dto.ErrorResponse "Failed to fetch groups"
// @Router /groups [get]
// @Security BearerAuth
func listPublicGroups(ctx *fiber.Ctx) error {
	selfQuery := ctx.Query("self")
	user := ctx.Locals(config.LOCALS_USER).(*models.User)

	var groups []dto.GroupResponse
	var err error

	switch selfQuery {
	case "creator":
		groups, err = groupsController.GetGroupsByCreator(user.ID)
	case "member":
		groups, err = groupUsersController.GetGroupsContainingMember(user.ID)
	default:
		groups, err = groupsController.GetPublicGroups(config.GroupsQueryLimit)
	}

	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	return ctx.Status(fiber.StatusOK).JSON(groups)
}

// @Summary Update an existing group
// @Description Update an existing group's details
// @Tags groups
// @ID update-group
// @Accept json
// @Produce json
// @Param groupIdOrCode path string true "Group ID or Code"
// @Param group body dto.UpdateGroupRequest true "Group Update Data"
// @Success 200 {object} dto.GroupResponse "Group updated successfully"
// @Failure 400 {object} dto.ErrorResponse "Invalid request"
// @Failure 404 {object} dto.ErrorResponse "Group not found"
// @Failure 422 {object} dto.ErrorResponse "Group info validation failed"
// @Failure 500 {object} dto.ErrorResponse "Failed to update group"
// @Router /groups/{groupIdOrCode} [patch]
// @Security BearerAuth
func updateGroup(ctx *fiber.Ctx) error {
	groupID := ctx.Params("groupIdOrCode")

	group, err := groupsController.GetGroupByIDorCode(groupID, false, false)
	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	req, parseError := parsers.ParseBody[dto.UpdateGroupRequest](ctx)
	if parseError != nil {
		return parsers.SendParsingError(ctx, parseError)
	}

	validateErr := validators.ValidateUpdateGroupRequest(req)
	if validateErr != nil {
		return validators.SendValidationError(ctx, validateErr)
	}

 // Check if place types are being updated to trigger place refresh
 placeTypesUpdated := len(req.PlaceTypes) > 0 && !slicesEqual(req.PlaceTypes, group.PlaceTypes)

	group, err = groupsController.UpdateGroup(group.ID, req)
	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	// If place types were updated, refresh places
	if placeTypesUpdated {
		go _triggerGroupMidpointUpdate(group)
	}

	return ctx.Status(fiber.StatusAccepted).JSON(group)
}

// @Summary Join a group
// @Description Join an existing group
// @Tags groups
// @ID join-group
// @Produce json
// @Param groupIdOrCode path string true "Group ID or Code"
// @Param groupUser body dto.GroupUserJoinRequest true "Group User"
// @Success 200 {object} dto.GroupUserResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request"
// @Failure 404 {object} dto.ErrorResponse "Group not found"
// @Failure 500 {object} dto.ErrorResponse "Failed to join group"
// @Router /groups/{groupIdOrCode}/join [put]
// @Security BearerAuth
func joinGroup(ctx *fiber.Ctx) error {
	user := ctx.Locals(config.LOCALS_USER).(*models.User)
	groupIDOrCode := ctx.Params("groupIdOrCode")

	group, err := groupsController.GetGroupByIDorCode(groupIDOrCode, false, false)
	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	groupUserReq, parseError := parsers.ParseBody[dto.GroupUserJoinRequest](ctx)
	if parseError != nil {
		return parsers.SendParsingError(ctx, parseError)
	}

	if group.MemberCount > 0 {
		// Validate if user is within max group join bounds
		validateErr := validators.ValidateLocationProximity(dto.Location{
			Latitude:  group.MidpointLatitude,
			Longitude: group.MidpointLongitude,
		}, dto.Location{
			Latitude:  groupUserReq.Latitude,
			Longitude: groupUserReq.Longitude,
		})
		if validateErr != nil {
			return validators.SendValidationError(ctx, validateErr)
		}
	}

	groupUserResp, err := groupUsersController.JoinGroup(group.ID, user.ID, groupUserReq)
	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	// TODO: can be done in side effect (goroutine)
	_triggerGroupMidpointUpdate(group)

	return ctx.Status(fiber.StatusAccepted).JSON(groupUserResp)
}

// @Summary Leave a group
// @Description Leave an existing group
// @Tags groups
// @ID leave-group
// @Produce json
// @Param groupIdOrCode path string true "Group ID or Code"
// @Success 200 {object} dto.GroupUserResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request"
// @Failure 404 {object} dto.ErrorResponse "Group not found"
// @Failure 500 {object} dto.ErrorResponse "Failed to leave group"
// @Router /groups/{groupIdOrCode}/join [delete]
// @Security BearerAuth
func leaveGroup(ctx *fiber.Ctx) error {
	user := ctx.Locals(config.LOCALS_USER).(*models.User)
	groupIDOrCode := ctx.Params("groupIdOrCode")

	group, err := groupsController.GetGroupByIDorCode(groupIDOrCode, false, false)
	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	err = groupUsersController.LeaveGroup(group.ID, user.ID)
	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	// TODO: can be done in side effect (goroutine)
	_triggerGroupMidpointUpdate(group)
	return ctx.Status(fiber.StatusAccepted).JSON([]byte("{}"))
}

// @Summary Get group information
// @Description Get details of a group by ID or code
// @Tags groups
// @ID get-group
// @Produce json
// @Param groupIdOrCode path string true "Group ID or Code"
// @Param includeUsers query bool false "Include Users"
// @Param includePlaces query bool false "Include Places"
// @Success 200 {object} dto.GroupResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request"
// @Failure 404 {object} dto.ErrorResponse "Group not found"
// @Failure 500 {object} dto.ErrorResponse "Failed to get group"
// @Router /groups/{groupIdOrCode} [get]
// @Security BearerAuth
func getGroup(ctx *fiber.Ctx) error {
	groupIDOrCode := ctx.Params("groupIdOrCode")
	includeUsers := ctx.QueryBool("includeUsers", false)
	includePlaces := ctx.QueryBool("includePlaces", false)

	group, err := groupsController.GetGroupByIDorCode(groupIDOrCode, includeUsers, includePlaces)
	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	return ctx.Status(fiber.StatusOK).JSON(group)
}

// side effects:
// 1. recalculate group midpoint
// 2. delete existing group places
// 3. populate group places (parallelly) for all place types
func _triggerGroupMidpointUpdate(group *dto.GroupResponse) {
	groupResp, err := _recalculateGroupMidpoint(group.ID)
	if err != nil {
		applogger.Error("Error recalculating group location", err)
	}
	err = _deleteExistingGroupPlaces(group.ID)
	if err != nil {
		applogger.Error("Error deleting existing group places", err)
	}
	for _, placeType := range groupResp.PlaceTypes {
		// TODO: can be parallelized?
		func(placeType config.PlaceType) {
			err := _populateGroupPlaces(groupResp, placeType)
			if err != nil {
				applogger.Error("Error populating group places", group.ID, "with type", placeType, err)
			}
		}(placeType)
	}
}

func _recalculateGroupMidpoint(groupID string) (*dto.GroupResponse, error) {
	applogger.Info("Recalculating group midpoint for group", groupID)
	centroidLatitude, centroidLongitude, err := groupUsersController.CalculateGroupCentroid(groupID)
	if err != nil {
		return nil, err
	}
	applogger.Info("Recalculated group midpoint for group", groupID, "to", centroidLatitude, centroidLongitude)

	groupMidpointUpdateRequest := &dto.UpdateGroupMidpointRequest{}
	groupMidpointUpdateRequest.Latitude = centroidLatitude
	groupMidpointUpdateRequest.Longitude = centroidLongitude

	groupResp, err := groupsController.UpdateGroupMidpoint(groupID, groupMidpointUpdateRequest)
	if err != nil {
		return nil, err
	}

	return groupResp, nil
}

func _deleteExistingGroupPlaces(groupID string) error {
	applogger.Warn("Deleting existing group places for group", groupID)
	err := groupPlacesController.RemoveAllPlacesFromGroup(groupID)
	if err != nil {
		return err
	}
	applogger.Warn("Deleted existing group places for group", groupID)
	return nil
}

func _populateGroupPlaces(group *dto.GroupResponse, placeType config.PlaceType) error {
	applogger.Info("Populating group places for group", group.ID, "with type", placeType)
	location := dto.Location{
		Latitude:  group.MidpointLatitude,
		Longitude: group.MidpointLongitude,
	}

	places, err := placesSearchService.NearbyPlaces(location, group.Radius, placeType)
	if err != nil {
		return err
	}
	groupPlacesAddRequest := &dto.GroupPlacesAddRequest{
		Places: places,
	}
	_, err = groupPlacesController.AddPlacesToGroup(group.ID, groupPlacesAddRequest)
	if err != nil {
		return err
	}
	return nil
}

// @Summary Create a new group
// @Description Create a new group with the authenticated user as the creator
// @Tags groups
// @ID create-group
// @Accept json
// @Produce json
// @Param group body dto.CreateGroupRequest true "Group Data"
// @Success 201 {object} dto.GroupResponse "Group created successfully"
// @Failure 400 {object} dto.ErrorResponse "Invalid request"
// @Failure 500 {object} dto.ErrorResponse "Failed to create group"
// @Router /groups [post]
// @Security BearerAuth
func createGroup(ctx *fiber.Ctx) error {
	user := ctx.Locals(config.LOCALS_USER).(*models.User)

	req, parseError := parsers.ParseBody[dto.CreateGroupRequest](ctx)
	if parseError != nil {
		return parsers.SendParsingError(ctx, parseError)
	}

	validateErr := validators.ValidateCreateGroupRequest(req)
	if validateErr != nil {
		return validators.SendValidationError(ctx, validateErr)
	}

	group, err := groupsController.CreateGroup(user.ID, req)
	if err != nil {
		return ctx.Status(err.(*fiber.Error).Code).JSON(dto.CreateErrorResponse(err.(*fiber.Error).Code, err.Error()))
	}

	return ctx.Status(fiber.StatusCreated).JSON(group)
}
