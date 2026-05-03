#include "SGameDepotStatusPanel.h"

#include "Styling/AppStyle.h"
#include "Widgets/Input/SButton.h"
#include "Widgets/Input/SSearchBox.h"
#include "Widgets/Layout/SBorder.h"
#include "Widgets/SBoxPanel.h"
#include "Widgets/Text/STextBlock.h"
#include "Widgets/Views/SHeaderRow.h"

#define LOCTEXT_NAMESPACE "SGameDepotStatusPanel"

namespace
{
TSharedRef<SWidget> MakeBadge(const FString& Text, const FSlateColor& Color)
{
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Recessed"))
        .Padding(FMargin(8.0f, 3.0f))
        [
            SNew(STextBlock)
            .Text(FText::FromString(Text))
            .ColorAndOpacity(Color)
            .Font(FAppStyle::GetFontStyle("SmallFontBold"))
        ];
}

FSlateColor ChangeColor(const FGameDepotAssetRow& Row)
{
    if (Row.Sync == EGameDepotSyncState::RoutingConflict)
    {
        return FSlateColor(FLinearColor(1.00f, 0.28f, 0.22f));
    }
    if (Row.Sync == EGameDepotSyncState::NewFile)
    {
        return FSlateColor(FLinearColor(0.35f, 0.62f, 1.00f));
    }
    if (Row.Sync == EGameDepotSyncState::MissingLocal || Row.Sync == EGameDepotSyncState::MissingRemote)
    {
        return FSlateColor(FLinearColor(1.00f, 0.52f, 0.25f));
    }
    return FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f));
}

FString RowStateText(const FGameDepotAssetRow& Row)
{
    if (!Row.DesiredRule.IsEmpty() && Row.DesiredRule != TEXT("-"))
    {
        return Row.DesiredRule;
    }
    return GameDepotStatusText::ToSyncText(Row.Sync);
}

class SGameDepotChangesRow : public SMultiColumnTableRow<FGameDepotAssetRowPtr>
{
public:
    SLATE_BEGIN_ARGS(SGameDepotChangesRow) {}
        SLATE_ARGUMENT(FGameDepotAssetRowPtr, Row)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs, const TSharedRef<STableViewBase>& OwnerTable)
    {
        Row = InArgs._Row;
        SMultiColumnTableRow<FGameDepotAssetRowPtr>::Construct(
            FSuperRowType::FArguments().Padding(FMargin(2.0f, 4.0f)), OwnerTable);
    }

    virtual TSharedRef<SWidget> GenerateWidgetForColumn(const FName& ColumnName) override
    {
        if (!Row.IsValid())
        {
            return SNew(STextBlock).Text(FText::GetEmpty());
        }

        if (ColumnName == FName(TEXT("State")))
        {
            return MakeBadge(RowStateText(*Row), ChangeColor(*Row));
        }
        if (ColumnName == FName(TEXT("Asset")))
        {
            return SNew(SVerticalBox)
                + SVerticalBox::Slot().AutoHeight()
                [
                    SNew(STextBlock)
                    .Text(FText::FromString(Row->AssetName.IsEmpty() ? Row->DepotPath : Row->AssetName))
                    .Font(FAppStyle::GetFontStyle(Row->Sync == EGameDepotSyncState::RoutingConflict ? "NormalFontBold" : "NormalFont"))
                ]
                + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 2.0f, 0.0f, 0.0f)
                [
                    SNew(STextBlock)
                    .Text(FText::FromString(Row->DepotPath))
                    .ColorAndOpacity(FSlateColor(FLinearColor(0.62f, 0.62f, 0.62f)))
                    .Font(FAppStyle::GetFontStyle("SmallFont"))
                ];
        }
        if (ColumnName == FName(TEXT("Reason")))
        {
            return SNew(SVerticalBox)
                + SVerticalBox::Slot().AutoHeight()
                [
                    SNew(STextBlock)
                    .Text(FText::FromString(Row->Message.IsEmpty() ? Row->Kind : Row->Message))
                    .AutoWrapText(true)
                ]
                + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 2.0f, 0.0f, 0.0f)
                [
                    SNew(STextBlock)
                    .Text(FText::FromString(Row->Kind.IsEmpty() ? Row->ShortHash : Row->Kind))
                    .ColorAndOpacity(FSlateColor(FLinearColor(0.55f, 0.55f, 0.55f)))
                    .Font(FAppStyle::GetFontStyle("SmallFont"))
                ];
        }
        return SNew(STextBlock).Text(FText::GetEmpty());
    }

private:
    FGameDepotAssetRowPtr Row;
};
}

void SGameDepotStatusPanel::Construct(const FArguments& InArgs)
{
    Provider = InArgs._Provider;
    OnRefreshRequested = InArgs._OnRefreshRequested;
    OnHistoryRequested = InArgs._OnHistoryRequested;
    OnRevertRequested = InArgs._OnRevertRequested;

    ApplyFilters();

    ChildSlot
    [
        SNew(SVerticalBox)
        + SVerticalBox::Slot().AutoHeight().Padding(8.0f)
        [
            BuildTopBar()
        ]
        + SVerticalBox::Slot().AutoHeight().Padding(8.0f, 0.0f, 8.0f, 8.0f)
        [
            BuildSummaryBar()
        ]
        + SVerticalBox::Slot().FillHeight(1.0f).Padding(8.0f, 0.0f, 8.0f, 8.0f)
        [
            SNew(SBorder)
            .BorderImage(FAppStyle::GetBrush("Brushes.Panel"))
            .Padding(6.0f)
            [
                SAssignNew(ListView, SListView<FGameDepotAssetRowPtr>)
                .ListItemsSource(&FilteredRows)
                .SelectionMode(ESelectionMode::Multi)
                .OnGenerateRow(this, &SGameDepotStatusPanel::MakeRowWidget)
                .HeaderRow
                (
                    SNew(SHeaderRow)
                    + SHeaderRow::Column(FName(TEXT("State"))).DefaultLabel(LOCTEXT("StateColumn", "State")).FixedWidth(120.0f)
                    + SHeaderRow::Column(FName(TEXT("Asset"))).DefaultLabel(LOCTEXT("AssetColumn", "Asset / Path")).FillWidth(0.45f)
                    + SHeaderRow::Column(FName(TEXT("Reason"))).DefaultLabel(LOCTEXT("ReasonColumn", "Reason")).FillWidth(0.55f)
                )
            ]
        ]
        + SVerticalBox::Slot().AutoHeight().Padding(8.0f, 0.0f, 8.0f, 8.0f)
        [
            BuildActionBar()
        ]
        + SVerticalBox::Slot().AutoHeight().Padding(8.0f, 0.0f, 8.0f, 8.0f)
        [
            BuildDetailsPanel()
        ]
    ];
}

void SGameDepotStatusPanel::RefreshRows()
{
    ApplyFilters();
    if (ListView.IsValid())
    {
        ListView->RequestListRefresh();
    }
}

void SGameDepotStatusPanel::FocusPath(const FString& DepotPath)
{
    ApplyFilters();
    if (!ListView.IsValid())
    {
        return;
    }
    for (const FGameDepotAssetRowPtr& Row : FilteredRows)
    {
        if (Row.IsValid() && Row->DepotPath == DepotPath)
        {
            ListView->SetSelection(Row);
            ListView->RequestScrollIntoView(Row);
            break;
        }
    }
}

TSharedRef<ITableRow> SGameDepotStatusPanel::MakeRowWidget(FGameDepotAssetRowPtr Row, const TSharedRef<STableViewBase>& OwnerTable)
{
    return SNew(SGameDepotChangesRow, OwnerTable).Row(Row);
}

TSharedRef<SWidget> SGameDepotStatusPanel::BuildTopBar()
{
    return SNew(SVerticalBox)
        + SVerticalBox::Slot().AutoHeight()
        [
            SNew(SHorizontalBox)
            + SHorizontalBox::Slot().FillWidth(1.0f)
            [
                SNew(SVerticalBox)
                + SVerticalBox::Slot().AutoHeight()
                [
                    SNew(STextBlock)
                    .Text(LOCTEXT("Title", "GameDepot Changes"))
                    .Font(FAppStyle::GetFontStyle("HeadingMedium"))
                ]
                + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 3.0f, 0.0f, 0.0f)
                [
                    SNew(STextBlock)
                    .Text(LOCTEXT("Subtitle", "Only local changes and conflicts are listed. Clean assets, hashes, and history are loaded on demand for speed."))
                    .ColorAndOpacity(FSlateColor(FLinearColor(0.65f, 0.65f, 0.65f)))
                ]
            ]
            + SHorizontalBox::Slot().AutoWidth().VAlign(VAlign_Center).Padding(6.0f, 0.0f)
            [
                SNew(SButton)
                .Text(LOCTEXT("Refresh", "Refresh"))
                .OnClicked_Lambda([this]()
                {
                    if (OnRefreshRequested.IsBound())
                    {
                        OnRefreshRequested.Execute();
                    }
                    RefreshRows();
                    return FReply::Handled();
                })
            ]
        ]
        + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 8.0f, 0.0f, 0.0f)
        [
            SAssignNew(SearchBox, SSearchBox)
            .HintText(LOCTEXT("SearchHint", "Search changed assets..."))
            .OnTextChanged_Lambda([this](const FText& Text)
            {
                SearchText = Text.ToString();
                RefreshRows();
            })
        ];
}

TSharedRef<SWidget> SGameDepotStatusPanel::BuildSummaryBar()
{
    const int32 Conflicts = Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::RoutingConflict) : 0;
    const int32 Added = Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::NewFile) : 0;
    const int32 Modified = Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::Modified) : 0;
    const int32 Deleted = Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::MissingRemote) : 0;
    const int32 Remote = Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::MissingLocal) : 0;

    return SNew(SHorizontalBox)
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Conflicts %d"), Conflicts), FSlateColor(FLinearColor(1.00f, 0.28f, 0.22f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Added %d"), Added), FSlateColor(FLinearColor(0.35f, 0.62f, 1.00f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Modified %d"), Modified), FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Deleted %d"), Deleted), FSlateColor(FLinearColor(1.00f, 0.52f, 0.25f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Remote %d"), Remote), FSlateColor(FLinearColor(0.60f, 0.68f, 1.00f)))]
        + SHorizontalBox::Slot().FillWidth(1.0f).VAlign(VAlign_Center).Padding(8.0f, 0.0f, 0.0f, 0.0f)
        [
            SNew(STextBlock)
            .Text(this, &SGameDepotStatusPanel::GetSummaryText)
            .ColorAndOpacity(FSlateColor(FLinearColor(0.65f, 0.65f, 0.65f)))
        ];
}

TSharedRef<SWidget> SGameDepotStatusPanel::BuildActionBar()
{
    return SNew(SHorizontalBox)
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 4.0f, 0.0f)
        [
            SNew(SButton)
            .Text(LOCTEXT("History", "History..."))
            .ToolTipText(LOCTEXT("HistoryTooltip", "Open history for the selected asset. History is queried on demand."))
            .OnClicked_Lambda([this]() { ShowHistoryForSelected(); return FReply::Handled(); })
        ]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 4.0f, 0.0f)
        [
            SNew(SButton)
            .Text(LOCTEXT("Revert", "Revert Local"))
            .ToolTipText(LOCTEXT("RevertTooltip", "Discard local unpublished changes for selected assets."))
            .OnClicked_Lambda([this]() { RevertSelectedUncommitted(); return FReply::Handled(); })
        ];
}

TSharedRef<SWidget> SGameDepotStatusPanel::BuildDetailsPanel() const
{
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Recessed"))
        .Padding(8.0f)
        [
            SNew(STextBlock)
            .Text(this, &SGameDepotStatusPanel::GetSelectionText)
            .AutoWrapText(true)
            .ColorAndOpacity(FSlateColor(FLinearColor(0.78f, 0.78f, 0.78f)))
        ];
}

void SGameDepotStatusPanel::ApplyFilters()
{
    FilteredRows.Reset();
    if (!Provider.IsValid())
    {
        return;
    }

    for (const FGameDepotAssetRowPtr& Row : Provider->GetRows())
    {
        if (PassesFilters(Row))
        {
            FilteredRows.Add(Row);
        }
    }
}

bool SGameDepotStatusPanel::PassesFilters(const FGameDepotAssetRowPtr& Row) const
{
    if (!Row.IsValid())
    {
        return false;
    }

    if (Row->Sync == EGameDepotSyncState::Synced || Row->Sync == EGameDepotSyncState::Ignored)
    {
        return false;
    }

    if (!SearchText.IsEmpty())
    {
        const FString Haystack = Row->DepotPath + TEXT(" ") + Row->AssetName + TEXT(" ") + Row->Message + TEXT(" ") + Row->Kind + TEXT(" ") + RowStateText(*Row);
        if (!Haystack.Contains(SearchText, ESearchCase::IgnoreCase))
        {
            return false;
        }
    }
    return true;
}

FText SGameDepotStatusPanel::GetSummaryText() const
{
    const int32 Total = Provider.IsValid() ? Provider->GetRows().Num() : 0;
    if (Total == 0)
    {
        return LOCTEXT("NoChanges", "No local changes or active conflicts. Update / Publish still performs exact verification.");
    }
    return FText::FromString(FString::Printf(TEXT("Showing %d / %d changed assets"), FilteredRows.Num(), Total));
}

FText SGameDepotStatusPanel::GetSelectionText() const
{
    const int32 SelectedCount = ListView.IsValid() ? ListView->GetNumItemsSelected() : 0;
    if (SelectedCount == 0)
    {
        return LOCTEXT("NoSelection", "This panel intentionally hides clean assets. Select a changed asset to open history or revert local unpublished changes.");
    }
    return FText::FromString(FString::Printf(TEXT("%d selected. History is loaded on demand; Revert Local only affects unpublished local changes."), SelectedCount));
}

TArray<FString> SGameDepotStatusPanel::GetSelectedDepotPaths() const
{
    TArray<FGameDepotAssetRowPtr> SelectedRows;
    if (ListView.IsValid())
    {
        ListView->GetSelectedItems(SelectedRows);
    }
    TArray<FString> Paths;
    for (const FGameDepotAssetRowPtr& Row : SelectedRows)
    {
        if (Row.IsValid())
        {
            Paths.Add(Row->DepotPath);
        }
    }
    return Paths;
}

FString SGameDepotStatusPanel::GetFirstSelectedDepotPath() const
{
    TArray<FGameDepotAssetRowPtr> SelectedRows;
    if (ListView.IsValid())
    {
        ListView->GetSelectedItems(SelectedRows);
    }
    for (const FGameDepotAssetRowPtr& Row : SelectedRows)
    {
        if (Row.IsValid())
        {
            return Row->DepotPath;
        }
    }
    return FString();
}

void SGameDepotStatusPanel::ShowHistoryForSelected()
{
    const FString DepotPath = GetFirstSelectedDepotPath();
    if (DepotPath.IsEmpty())
    {
        return;
    }
    if (OnHistoryRequested.IsBound())
    {
        OnHistoryRequested.Execute(DepotPath);
    }
}

void SGameDepotStatusPanel::RevertSelectedUncommitted()
{
    const TArray<FString> SelectedPaths = GetSelectedDepotPaths();
    if (SelectedPaths.Num() == 0)
    {
        return;
    }
    if (OnRevertRequested.IsBound())
    {
        OnRevertRequested.Execute(SelectedPaths);
    }
    else if (Provider.IsValid())
    {
        Provider->RevertUncommittedChanges(SelectedPaths);
        RefreshRows();
    }
}

#undef LOCTEXT_NAMESPACE
