from fastapi import APIRouter, Depends

from rekenraam_api.api.dependencies import get_metadata_service
from rekenraam_api.schemas.metadata import (
    CategorySummary,
    CommoditySummary,
    CountrySummary,
    InstitutionSummary,
    PayeeSummary,
    PersonSummary,
    ProjectSummary,
    TagSummary,
)
from rekenraam_api.services.metadata import MetadataService


router = APIRouter(tags=["metadata"])


@router.get("/commodities", response_model=list[CommoditySummary])
async def list_commodities(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[CommoditySummary]:
    return await metadata_service.list_commodities()


@router.get("/countries", response_model=list[CountrySummary])
async def list_countries(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[CountrySummary]:
    return await metadata_service.list_countries()


@router.get("/institutions", response_model=list[InstitutionSummary])
async def list_institutions(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[InstitutionSummary]:
    return await metadata_service.list_institutions()


@router.get("/categories", response_model=list[CategorySummary])
async def list_categories(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[CategorySummary]:
    return await metadata_service.list_categories()


@router.get("/payees", response_model=list[PayeeSummary])
async def list_payees(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[PayeeSummary]:
    return await metadata_service.list_payees()


@router.get("/tags", response_model=list[TagSummary])
async def list_tags(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[TagSummary]:
    return await metadata_service.list_tags()


@router.get("/people", response_model=list[PersonSummary])
async def list_people(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[PersonSummary]:
    return await metadata_service.list_people()


@router.get("/projects", response_model=list[ProjectSummary])
async def list_projects(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[ProjectSummary]:
    return await metadata_service.list_projects()