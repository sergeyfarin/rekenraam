from fastapi import APIRouter, Depends, HTTPException, status

from rekenraam_api.api.dependencies import get_metadata_service
from rekenraam_api.schemas.metadata import (
    CategoryCreateInput,
    CategorySummary,
    CategoryUpdateInput,
    CommoditySummary,
    CountrySummary,
    InstitutionSummary,
    PayeeCreateInput,
    PayeeSummary,
    PayeeUpdateInput,
    PersonCreateInput,
    PersonSummary,
    ProjectCreateInput,
    ProjectSummary,
    TagCreateInput,
    TagSummary,
    TagUpdateInput,
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


@router.put("/categories/{category_id}", response_model=CategorySummary)
async def update_category(
    category_id: int,
    input: CategoryUpdateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> CategorySummary:
    try:
        category = await metadata_service.update_category(category_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if category is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="category not found")
    return category


@router.delete("/categories/{category_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_category(
    category_id: int,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> None:
    try:
        deleted = await metadata_service.delete_category(category_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(error)) from error

    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="category not found")


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


@router.put("/payees/{payee_id}", response_model=PayeeSummary)
async def update_payee(
    payee_id: int,
    input: PayeeUpdateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> PayeeSummary:
    try:
        payee = await metadata_service.update_payee(payee_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if payee is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="payee not found")
    return payee


@router.delete("/payees/{payee_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_payee(
    payee_id: int,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> None:
    try:
        deleted = await metadata_service.delete_payee(payee_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(error)) from error

    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="payee not found")


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


@router.put("/tags/{tag_id}", response_model=TagSummary)
async def update_tag(
    tag_id: int,
    input: TagUpdateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> TagSummary:
    try:
        tag = await metadata_service.update_tag(tag_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if tag is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="tag not found")
    return tag


@router.delete("/tags/{tag_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_tag(
    tag_id: int,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> None:
    try:
        deleted = await metadata_service.delete_tag(tag_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(error)) from error

    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="tag not found")


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