from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_ergonomics_service
from rekenraam_api.schemas.ergonomics import UserPreferenceSummary, UserPreferenceUpdateInput
from rekenraam_api.services.access import AuthorizationError
from rekenraam_api.services.ergonomics import ErgonomicsService

router = APIRouter(prefix="/preferences", tags=["preferences"])


@router.get("", response_model=UserPreferenceSummary)
async def get_preferences(
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> UserPreferenceSummary:
    return await ergonomics_service.get_preferences()


@router.put("", response_model=UserPreferenceSummary)
async def update_preferences(
    input: UserPreferenceUpdateInput,
    ergonomics_service: ErgonomicsService = Depends(get_ergonomics_service),
) -> UserPreferenceSummary:
    try:
        return await ergonomics_service.update_preferences(input)
    except AuthorizationError as error:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail=str(error)) from error
