#include "SGameDepotStatusPanel.h"

#include "SGameDepotHistoryDialog.h"

#include "Framework/Application/SlateApplication.h"
#include "Framework/MultiBox/MultiBoxBuilder.h"
#include "Styling/AppStyle.h"
#include "Widgets/Input/SButton.h"
#include "Widgets/SWindow.h"
#include "Widgets/Input/SComboBox.h"
#include "Widgets/Input/SSearchBox.h"
#include "Widgets/Layout/SBorder.h"
#include "Widgets/Layout/SScrollBox.h"
#include "Widgets/Layout/SSeparator.h"
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

class SGameDepotStatusRow : public SMultiColumnTableRow<FGameDepotAssetRowPtr>
{
public:
    SLATE_BEGIN_ARGS(SGameDepotStatusRow) {}
        SLATE_ARGUMENT(FGameDepotAssetRowPtr, Row)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs, const TSharedRef<STableViewBase>& OwnerTable)
    {
        Row = InArgs._Row;
        SMultiColumnTableRow<FGameDepotAssetRowPtr>::Construct(
            FSuperRowType::FArguments().Padding(FMargin(2.0f, 3.0f)), OwnerTable);
    }

    virtual TSharedRef<SWidget> GenerateWidgetForColumn(const FName& ColumnName) override
    {
        if (!Row.IsValid())
        {
            return SNew(STextBlock).Text(FText::GetEmpty());
        }

        if (ColumnName == FName(TEXT("Asset")))
        {
            return SNew(SVerticalBox)
                + SVerticalBox::Slot().AutoHeight()
                [
                    SNew(STextBlock)
                    .Text(FText::FromString(Row->bHistoryOnly ? FString::Printf(TEXT("[History only] %s"), *(Row->AssetName.IsEmpty() ? Row->DepotPath : Row->AssetName)) : (Row->AssetName.IsEmpty() ? Row->DepotPath : Row->AssetName)))
                    .Font(FAppStyle::GetFontStyle(Row->bSelected ? "NormalFontBold" : "NormalFont"))
                ]
                + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 2.0f, 0.0f, 0.0f)
                [
                    SNew(STextBlock)
                    .Text(FText::FromString(Row->DepotPath))
                    .ColorAndOpacity(FSlateColor(FLinearColor(0.62f, 0.62f, 0.62f)))
                    .Font(FAppStyle::GetFontStyle("SmallFont"))
                ];
        }
        if (ColumnName == FName(TEXT("Storage")))
        {
            return MakeBadge(GameDepotStatusText::ToStorageText(Row->Storage), GameDepotStatusText::ToStorageColor(Row->Storage));
        }
        if (ColumnName == FName(TEXT("Sync")))
        {
            return MakeBadge(GameDepotStatusText::ToSyncText(Row->Sync), GameDepotStatusText::ToSyncColor(Row->Sync));
        }
        if (ColumnName == FName(TEXT("Rule")))
        {
            return SNew(STextBlock)
                .Text(FText::FromString(Row->DesiredRule))
                .ColorAndOpacity(FSlateColor(FLinearColor(0.85f, 0.85f, 0.85f)));
        }
        if (ColumnName == FName(TEXT("Remote")))
        {
            return SNew(STextBlock)
                .Text(FText::FromString(Row->RemoteState))
                .ColorAndOpacity(Row->bRemoteExists ? FSlateColor(FLinearColor(0.20f, 0.78f, 0.36f)) : FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f)));
        }
        if (ColumnName == FName(TEXT("Hash")))
        {
            return SNew(STextBlock)
                .Text(FText::FromString(Row->ShortHash))
                .ColorAndOpacity(FSlateColor(FLinearColor(0.55f, 0.55f, 0.55f)))
                .Font(FAppStyle::GetFontStyle("SmallFont"));
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
    OnRuleRequested = InArgs._OnRuleRequested;
    OnHistoryRequested = InArgs._OnHistoryRequested;
    OnRevertRequested = InArgs._OnRevertRequested;
    OnHistoryRestored = InArgs._OnHistoryRestored;

    StorageOptions = {
        MakeShared<FString>(TEXT("All Storage")),
        MakeShared<FString>(TEXT("Git")),
        MakeShared<FString>(TEXT("OSS")),
        MakeShared<FString>(TEXT("New")),
        MakeShared<FString>(TEXT("History")),
        MakeShared<FString>(TEXT("Review")),
        MakeShared<FString>(TEXT("Ignored"))
    };
    SyncOptions = {
        MakeShared<FString>(TEXT("All Sync")),
        MakeShared<FString>(TEXT("Synced")),
        MakeShared<FString>(TEXT("Modified")),
        MakeShared<FString>(TEXT("Missing")),
        MakeShared<FString>(TEXT("New")),
        MakeShared<FString>(TEXT("Needs Rule")),
        MakeShared<FString>(TEXT("Conflict"))
    };
    SelectedStorageOption = StorageOptions[0];
    SelectedSyncOption = SyncOptions[0];

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
                    + SHeaderRow::Column(FName(TEXT("Asset"))).DefaultLabel(LOCTEXT("AssetColumn", "Asset / Path")).FillWidth(0.30f)
                    + SHeaderRow::Column(FName(TEXT("Storage"))).DefaultLabel(LOCTEXT("StorageColumn", "Storage")).FixedWidth(108.0f)
                    + SHeaderRow::Column(FName(TEXT("Sync"))).DefaultLabel(LOCTEXT("SyncColumn", "Sync")).FixedWidth(124.0f)
                    + SHeaderRow::Column(FName(TEXT("Rule"))).DefaultLabel(LOCTEXT("RuleColumn", "Next Rule")).FixedWidth(90.0f)
                    + SHeaderRow::Column(FName(TEXT("Remote"))).DefaultLabel(LOCTEXT("RemoteColumn", "OSS")).FixedWidth(90.0f)
                    + SHeaderRow::Column(FName(TEXT("Hash"))).DefaultLabel(LOCTEXT("HashColumn", "Hash")).FixedWidth(82.0f)
                    + SHeaderRow::Column(FName(TEXT("Message"))).DefaultLabel(LOCTEXT("MessageColumn", "Message")).FillWidth(0.42f)
                )
            ]
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
    return SNew(SGameDepotStatusRow, OwnerTable).Row(Row);
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
                    .Text(LOCTEXT("Title", "GameDepot Asset Status"))
                    .Font(FAppStyle::GetFontStyle("HeadingMedium"))
                ]
                + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 3.0f, 0.0f, 0.0f)
                [
                    SNew(STextBlock)
                    .Text(LOCTEXT("Subtitle", "Daemon mode: status, rules, history, submit and sync are served by the GameDepot daemon. Mock mode is still available from plugin settings."))
                    .ColorAndOpacity(FSlateColor(FLinearColor(0.65f, 0.65f, 0.65f)))
                ]
            ]
            + SHorizontalBox::Slot().AutoWidth().VAlign(VAlign_Center).Padding(6.0f, 0.0f)
            [
                SNew(SButton)
                .Text(LOCTEXT("Refresh", "Refresh Status"))
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
            SNew(SHorizontalBox)
            + SHorizontalBox::Slot().FillWidth(1.0f).VAlign(VAlign_Center)
            [
                SAssignNew(SearchBox, SSearchBox)
                .HintText(LOCTEXT("SearchHint", "Search path, asset, rule, sync state..."))
                .OnTextChanged_Lambda([this](const FText& Text)
                {
                    SearchText = Text.ToString();
                    RefreshRows();
                })
            ]
            + SHorizontalBox::Slot().AutoWidth().Padding(8.0f, 0.0f, 0.0f, 0.0f)
            [
                BuildFilterCombo(StorageOptions, true)
            ]
            + SHorizontalBox::Slot().AutoWidth().Padding(6.0f, 0.0f, 0.0f, 0.0f)
            [
                BuildFilterCombo(SyncOptions, false)
            ]
            + SHorizontalBox::Slot().AutoWidth().Padding(10.0f, 0.0f, 0.0f, 0.0f)
            [
                BuildRuleButtons()
            ]
        ];
}

TSharedRef<SWidget> SGameDepotStatusPanel::BuildSummaryBar()
{
    return SNew(SHorizontalBox)
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Git %d"), Provider.IsValid() ? Provider->CountByStorage(EGameDepotStorage::Git) + Provider->CountByStorage(EGameDepotStorage::NewToGit) : 0), FSlateColor(FLinearColor(0.42f, 0.75f, 1.00f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("OSS %d"), Provider.IsValid() ? Provider->CountByStorage(EGameDepotStorage::OSS) + Provider->CountByStorage(EGameDepotStorage::NewToOSS) : 0), FSlateColor(FLinearColor(0.86f, 0.56f, 1.00f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Modified %d"), Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::Modified) : 0), FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Missing %d"), Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::MissingLocal) + Provider->CountBySync(EGameDepotSyncState::MissingRemote) : 0), FSlateColor(FLinearColor(1.00f, 0.28f, 0.22f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("History %d"), Provider.IsValid() ? Provider->CountHistoryOnly() : 0), FSlateColor(FLinearColor(0.78f, 0.68f, 1.00f)))]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 6.0f, 0.0f)
        [MakeBadge(FString::Printf(TEXT("Review %d"), Provider.IsValid() ? Provider->CountBySync(EGameDepotSyncState::ReviewRequired) : 0), FSlateColor(FLinearColor(1.00f, 0.74f, 0.25f)))]
        + SHorizontalBox::Slot().FillWidth(1.0f).VAlign(VAlign_Center).Padding(8.0f, 0.0f, 0.0f, 0.0f)
        [
            SNew(STextBlock)
            .Text(this, &SGameDepotStatusPanel::GetSummaryText)
            .ColorAndOpacity(FSlateColor(FLinearColor(0.65f, 0.65f, 0.65f)))
        ];
}

TSharedRef<SWidget> SGameDepotStatusPanel::BuildFilterCombo(TArray<TSharedPtr<FString>>& Options, bool bStorageFilterCombo)
{
    const TSharedPtr<FString> Initial = bStorageFilterCombo ? SelectedStorageOption : SelectedSyncOption;
    return SNew(SComboBox<TSharedPtr<FString>>)
        .OptionsSource(&Options)
        .InitiallySelectedItem(Initial)
        .OnGenerateWidget_Lambda([](TSharedPtr<FString> Item)
        {
            return SNew(STextBlock).Text(FText::FromString(Item.IsValid() ? *Item : TEXT("")));
        })
        .OnSelectionChanged_Lambda([this, bStorageFilterCombo](TSharedPtr<FString> Item, ESelectInfo::Type)
        {
            if (bStorageFilterCombo)
            {
                SelectedStorageOption = Item;
                StorageFilter = Item.IsValid() ? *Item : TEXT("All Storage");
            }
            else
            {
                SelectedSyncOption = Item;
                SyncFilter = Item.IsValid() ? *Item : TEXT("All Sync");
            }
            RefreshRows();
        })
        [
            SNew(STextBlock)
            .Text_Lambda([this, bStorageFilterCombo]()
            {
                const TSharedPtr<FString> Selected = bStorageFilterCombo ? SelectedStorageOption : SelectedSyncOption;
                return FText::FromString(Selected.IsValid() ? *Selected : TEXT(""));
            })
        ];
}


TSharedRef<SWidget> SGameDepotStatusPanel::BuildRuleButtons()
{
    return SNew(SHorizontalBox)
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 4.0f, 0.0f)
        [
            SNew(SButton)
            .Text(LOCTEXT("History", "History..."))
            .ToolTipText(LOCTEXT("HistoryTooltip", "Open history for the selected row. This also works for history-only files that are not in the current Content Browser."))
            .OnClicked_Lambda([this]() { ShowHistoryForSelected(); return FReply::Handled(); })
        ]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 4.0f, 0.0f)
        [
            SNew(SButton)
            .Text(LOCTEXT("Revert", "Revert"))
            .ToolTipText(LOCTEXT("RevertTooltip", "Discard unsubmitted changes for selected rows."))
            .OnClicked_Lambda([this]() { RevertSelectedUncommitted(); return FReply::Handled(); })
        ]
        + SHorizontalBox::Slot().AutoWidth().Padding(8.0f, 0.0f, 4.0f, 0.0f)
        [
            SNew(SButton)
            .Text(LOCTEXT("RuleOSS", "Set OSS"))
            .OnClicked_Lambda([this]() { RequestRuleForSelected(TEXT("blob")); return FReply::Handled(); })
        ]
        + SHorizontalBox::Slot().AutoWidth().Padding(0.0f, 0.0f, 4.0f, 0.0f)
        [
            SNew(SButton)
            .Text(LOCTEXT("RuleGit", "Set Git"))
            .OnClicked_Lambda([this]() { RequestRuleForSelected(TEXT("git")); return FReply::Handled(); })
        ]
        + SHorizontalBox::Slot().AutoWidth()
        [
            SNew(SButton)
            .Text(LOCTEXT("RuleIgnore", "Ignore"))
            .OnClicked_Lambda([this]() { RequestRuleForSelected(TEXT("ignore")); return FReply::Handled(); })
        ];
}

TSharedRef<SWidget> SGameDepotStatusPanel::BuildLegend() const
{
    return SNew(STextBlock)
        .Text(LOCTEXT("Legend", "Legend: Storage = manifest route in current version or next-rule route for new files. History-only rows are files absent from the current Content Browser but found by traversing older Git commits and manifests. Select one and use History... to restore it."))
        .AutoWrapText(true)
        .ColorAndOpacity(FSlateColor(FLinearColor(0.62f, 0.62f, 0.62f)));
}

TSharedRef<SWidget> SGameDepotStatusPanel::BuildDetailsPanel() const
{
    return SNew(SBorder)
        .BorderImage(FAppStyle::GetBrush("Brushes.Recessed"))
        .Padding(8.0f)
        [
            SNew(SVerticalBox)
            + SVerticalBox::Slot().AutoHeight()
            [
                SNew(STextBlock)
                .Text(this, &SGameDepotStatusPanel::GetSelectionText)
                .ColorAndOpacity(FSlateColor(FLinearColor(0.78f, 0.78f, 0.78f)))
            ]
            + SVerticalBox::Slot().AutoHeight().Padding(0.0f, 5.0f, 0.0f, 0.0f)
            [
                BuildLegend()
            ]
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

    if (!SearchText.IsEmpty())
    {
        const FString Haystack = Row->DepotPath + TEXT(" ") + Row->AssetName + TEXT(" ") + Row->DesiredRule + TEXT(" ") + GameDepotStatusText::ToSyncText(Row->Sync) + TEXT(" ") + GameDepotStatusText::ToStorageText(Row->Storage) + TEXT(" ") + Row->Message + (Row->bHistoryOnly ? TEXT(" history deleted absent") : TEXT(""));
        if (!Haystack.Contains(SearchText, ESearchCase::IgnoreCase))
        {
            return false;
        }
    }

    const FString StorageValue = SelectedStorageOption.IsValid() ? *SelectedStorageOption : TEXT("All Storage");
    if (StorageValue == TEXT("Git") && !(Row->Storage == EGameDepotStorage::Git || Row->Storage == EGameDepotStorage::NewToGit)) return false;
    if (StorageValue == TEXT("OSS") && !(Row->Storage == EGameDepotStorage::OSS || Row->Storage == EGameDepotStorage::NewToOSS)) return false;
    if (StorageValue == TEXT("New") && !(Row->Storage == EGameDepotStorage::NewToGit || Row->Storage == EGameDepotStorage::NewToOSS || Row->Sync == EGameDepotSyncState::NewFile)) return false;
    if (StorageValue == TEXT("History") && !Row->bHistoryOnly) return false;
    if (StorageValue == TEXT("Review") && Row->Storage != EGameDepotStorage::Review) return false;
    if (StorageValue == TEXT("Ignored") && Row->Storage != EGameDepotStorage::Ignored) return false;

    const FString SyncValue = SelectedSyncOption.IsValid() ? *SelectedSyncOption : TEXT("All Sync");
    if (SyncValue == TEXT("Synced") && Row->Sync != EGameDepotSyncState::Synced) return false;
    if (SyncValue == TEXT("Modified") && Row->Sync != EGameDepotSyncState::Modified) return false;
    if (SyncValue == TEXT("Missing") && !(Row->Sync == EGameDepotSyncState::MissingLocal || Row->Sync == EGameDepotSyncState::MissingRemote)) return false;
    if (SyncValue == TEXT("New") && Row->Sync != EGameDepotSyncState::NewFile) return false;
    if (SyncValue == TEXT("Needs Rule") && Row->Sync != EGameDepotSyncState::ReviewRequired) return false;
    if (SyncValue == TEXT("Conflict") && Row->Sync != EGameDepotSyncState::RoutingConflict) return false;

    return true;
}

FText SGameDepotStatusPanel::GetSummaryText() const
{
    const int32 Total = Provider.IsValid() ? Provider->GetRows().Num() : 0;
    return FText::FromString(FString::Printf(TEXT("Showing %d / %d assets"), FilteredRows.Num(), Total));
}

FText SGameDepotStatusPanel::GetSelectionText() const
{
    const int32 SelectedCount = ListView.IsValid() ? ListView->GetNumItemsSelected() : 0;
    if (SelectedCount == 0)
    {
        return LOCTEXT("NoSelection", "Select rows to preview batch actions. History-only rows are recoverable files that are not currently visible in Content Browser; select one and click History... to restore.");
    }
    return FText::FromString(FString::Printf(TEXT("%d selected. Use History... to restore old Git/OSS versions, Revert for local changes, or rule buttons to update routing."), SelectedCount));
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
        return;
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
void SGameDepotStatusPanel::RequestRuleForSelected(const FString& Mode)
{
    const TArray<FString> SelectedPaths = GetSelectedDepotPaths();
    if (SelectedPaths.Num() == 0)
    {
        return;
    }
    if (OnRuleRequested.IsBound())
    {
        OnRuleRequested.Execute(SelectedPaths, Mode);
    }
    if (Provider.IsValid())
    {
        Provider->SetRuleForDepotPaths(SelectedPaths, Mode);
    }
    RefreshRows();
}

#undef LOCTEXT_NAMESPACE
