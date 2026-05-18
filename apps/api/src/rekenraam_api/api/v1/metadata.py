from fastapi import APIRouter, Depends, HTTPException, Query, status

from rekenraam_api.api.dependencies import get_metadata_service
from rekenraam_api.schemas.metadata import (
    CategoryCreateInput,
    CategorySummary,
    CategoryUpdateInput,
    CommodityAutocompleteOption,
    CommoditySummary,
    CommodityUpdateInput,
    CountrySummary,
    CurrencyActivationInput,
    CurrencyCreateInput,
    CurrencySummary,
    CurrencyUpdateInput,
    InstitutionCreateInput,
    InstitutionSummary,
    InstitutionUpdateInput,
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


@router.get("/commodities/autocomplete", response_model=list[CommodityAutocompleteOption])
async def autocomplete_commodities(
    query: str = Query(min_length=1),
    book_id: int = Query(default=1),
    limit: int = Query(default=12, ge=1, le=50),
    active_only: bool = Query(default=False),
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[CommodityAutocompleteOption]:
    return await metadata_service.autocomplete_commodities(
        book_id=book_id,
        query=query,
        limit=limit,
        active_only=active_only,
    )


@router.get("/commodities", response_model=list[CommoditySummary])
async def list_commodities(
    book_id: int = Query(default=1),
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[CommoditySummary]:
    return await metadata_service.list_commodities(book_id)


@router.put("/commodities/{commodity_id}", response_model=CommoditySummary)
async def update_commodity(
    commodity_id: int,
    input: CommodityUpdateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> CommoditySummary:
    try:
        commodity = await metadata_service.update_commodity(commodity_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if commodity is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="commodity not found")
    return commodity


@router.get("/currencies", response_model=list[CurrencySummary])
async def list_currencies(
    book_id: int = 1,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[CurrencySummary]:
    return await metadata_service.list_currencies(book_id)


@router.post("/currencies", response_model=CurrencySummary)
async def create_currency(
    input: CurrencyCreateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> CurrencySummary:
    try:
        return await metadata_service.create_currency(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/currencies/{currency_id}", response_model=CurrencySummary)
async def update_currency(
    currency_id: int,
    input: CurrencyUpdateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> CurrencySummary:
    try:
        currency = await metadata_service.update_currency(currency_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if currency is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="currency not found")
    return currency


@router.post("/currencies/{currency_id}/default", response_model=CurrencySummary)
async def set_default_currency(
    currency_id: int,
    book_id: int = 1,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> CurrencySummary:
    try:
        currency = await metadata_service.set_default_currency(
            book_id=book_id, currency_id=currency_id
        )
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if currency is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="currency not found")
    return currency


@router.post("/currencies/{currency_id}/activation", response_model=CurrencySummary)
async def set_currency_active(
    currency_id: int,
    input: CurrencyActivationInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> CurrencySummary:
    try:
        currency = await metadata_service.set_currency_active(currency_id=currency_id, input=input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if currency is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="currency not found")
    return currency


@router.get("/countries", response_model=list[CountrySummary])
async def list_countries(
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[CountrySummary]:
    return await metadata_service.list_countries()


@router.get("/institutions", response_model=list[InstitutionSummary])
async def list_institutions(
    book_id: int = Query(default=1),
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[InstitutionSummary]:
    return await metadata_service.list_institutions(book_id)


@router.post("/institutions", response_model=InstitutionSummary)
async def create_institution(
    input: InstitutionCreateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> InstitutionSummary:
    try:
        return await metadata_service.create_institution(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error


@router.put("/institutions/{institution_id}", response_model=InstitutionSummary)
async def update_institution(
    institution_id: int,
    input: InstitutionUpdateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> InstitutionSummary:
    try:
        institution = await metadata_service.update_institution(institution_id, input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error

    if institution is None:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="institution not found")
    return institution


@router.delete("/institutions/{institution_id}", status_code=status.HTTP_204_NO_CONTENT)
async def delete_institution(
    institution_id: int,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> None:
    try:
        deleted = await metadata_service.delete_institution(institution_id)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(error)) from error

    if not deleted:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="institution not found")


@router.get("/categories", response_model=list[CategorySummary])
async def list_categories(
    book_id: int = Query(default=1),
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[CategorySummary]:
    return await metadata_service.list_categories(book_id)


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
    book_id: int = Query(default=1),
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[PayeeSummary]:
    return await metadata_service.list_payees(book_id)


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
    book_id: int = Query(default=1),
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[TagSummary]:
    return await metadata_service.list_tags(book_id)


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
    book_id: int = Query(default=1),
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[PersonSummary]:
    return await metadata_service.list_people(book_id)


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
    book_id: int = Query(default=1),
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> list[ProjectSummary]:
    return await metadata_service.list_projects(book_id)


@router.post("/projects", response_model=ProjectSummary)
async def create_project(
    input: ProjectCreateInput,
    metadata_service: MetadataService = Depends(get_metadata_service),
) -> ProjectSummary:
    try:
        return await metadata_service.create_project(input)
    except ValueError as error:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail=str(error)) from error
