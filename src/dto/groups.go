package dto

import "github.com/championswimmer/api.midpoint.place/src/config"

type CreateGroupRequest struct {
	Name       string              `json:"name" validate:"required"`
	Type       config.GroupType    `json:"type" validate:"omitempty,oneof=public protected private"`
	Secret     string              `json:"secret" validate:"omitempty"`
	Radius     int                 `json:"radius" validate:"omitempty,min=0"`
	PlaceTypes []config.PlaceType  `json:"place_types" validate:"omitempty"`
}

type UpdateGroupRequest struct {
	Name       string              `json:"name" validate:"omitempty"`
	Type       config.GroupType    `json:"type" validate:"omitempty,oneof=public protected private"`
	Secret     string              `json:"secret" validate:"omitempty"`
	Radius     int                 `json:"radius" validate:"omitempty,min=0"`
	PlaceTypes []config.PlaceType  `json:"place_types" validate:"omitempty"`
}

type UpdateGroupMidpointRequest struct {
	Location
}

type GroupCreator struct {
	ID          uint   `json:"id"`
	DisplayName string `json:"display_name"`
}

type GroupResponse struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Type              config.GroupType     `json:"type"`
	Code              string               `json:"code"`
	Creator           GroupCreator         `json:"creator"`
	MidpointLatitude  float64              `json:"midpoint_latitude"`
	MidpointLongitude float64              `json:"midpoint_longitude"`
	Radius            int                  `json:"radius"`
	PlaceTypes        []config.PlaceType   `json:"place_types"`
	MemberCount       int                  `json:"member_count,omitempty"`
	Members           []GroupUserResponse  `json:"members,omitempty"`
	Places            []GroupPlaceResponse `json:"places,omitempty"`
}
