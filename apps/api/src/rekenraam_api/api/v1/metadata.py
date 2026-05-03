from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_metadata_service
from rekenraam_api.schemas.metadata import (
    CategoryCreateInput,
    CategorySummary,
    CommoditySummary,
    CountrySummary,
    InstitutionSummary,
    PayeeCreateInput,
    PayeeSummary,
    PersonCreateInput,
    PersonSummary,
    ProjectCreateInput,
    ProjectSummary,
    TagCreateInput,
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


@router.post("/categories", response_model=CategorySummary)
async def create_category(
    input: CategoryCreateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> CategorySummary:
    try:
        return await metadata_service.create_category(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.get("/payees", response_model=list[PayeeSummary])
async def list_payees(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[PayeeSummary]:
    return await metadata_service.list_payees()


@router.post("/payees", response_model=PayeeSummary)
async def create_payee(
    input: PayeeCreateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> PayeeSummary:
    try:
        return await metadata_service.create_payee(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.get("/tags", response_model=list[TagSummary])
async def list_tags(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[TagSummary]:
    return await metadata_service.list_tags()


@router.post("/tags", response_model=TagSummary)
async def create_tag(
    input: TagCreateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> TagSummary:
    try:
        return await metadata_service.create_tag(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.get("/people", response_model=list[PersonSummary])
async def list_people(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[PersonSummary]:
    return await metadata_service.list_people()


@router.post("/people", response_model=PersonSummary)
async def create_person(
    input: PersonCreateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> PersonSummary:
    try:
        return await metadata_service.create_person(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.get("/projects", response_model=list[ProjectSummary])
async def list_projects(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[ProjectSummary]:
    return await metadata_service.list_projects()


@router.post("/projects", response_model=ProjectSummary)
async def create_project(
    input: ProjectCreateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> ProjectSummary:
    try:
        return await metadata_service.create_project(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error