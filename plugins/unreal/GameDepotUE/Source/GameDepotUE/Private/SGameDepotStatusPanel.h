#pragma once

#include "CoreMinimal.h"
#include "Widgets/SCompoundWidget.h"
#include "Widgets/Views/SListView.h"
#include "GameDepotMockStatusProvider.h"

class SSearchBox;

DECLARE_DELEGATE_TwoParams(FGameDepotRulePathsAction, const TArray<FString>& /*Paths*/, const FString& /*Mode*/);
DECLARE_DELEGATE_OneParam(FGameDepotPathAction, const FString& /*DepotPath*/);
DECLARE_DELEGATE_OneParam(FGameDepotPathsAction, const TArray<FString>& /*Paths*/);

class SGameDepotStatusPanel : public SCompoundWidget
{
public:
    SLATE_BEGIN_ARGS(SGameDepotStatusPanel) {}
        SLATE_ARGUMENT(TSharedPtr<FGameDepotMockStatusProvider>, Provider)
        SLATE_EVENT(FSimpleDelegate, OnRefreshRequested)
        SLATE_EVENT(FGameDepotRulePathsAction, OnRuleRequested)
        SLATE_EVENT(FGameDepotPathAction, OnHistoryRequested)
        SLATE_EVENT(FGameDepotPathsAction, OnRevertRequested)
        SLATE_EVENT(FGameDepotPathAction, OnHistoryRestored)
    SLATE_END_ARGS()

    void Construct(const FArguments& InArgs);
    void RefreshRows();
    void FocusPath(const FString& DepotPath);

private:
    TSharedPtr<FGameDepotMockStatusProvider> Provider;
    TArray<FGameDepotAssetRowPtr> FilteredRows;
    TSharedPtr<SListView<FGameDepotAssetRowPtr>> ListView;
    TSharedPtr<SSearchBox> SearchBox;
    FString SearchText;
    FString StorageFilter;
    FString SyncFilter;
    FSimpleDelegate OnRefreshRequested;
    FGameDepotRulePathsAction OnRuleRequested;
    FGameDepotPathAction OnHistoryRequested;
    FGameDepotPathsAction OnRevertRequested;
    FGameDepotPathAction OnHistoryRestored;

    TArray<TSharedPtr<FString>> StorageOptions;
    TArray<TSharedPtr<FString>> SyncOptions;
    TSharedPtr<FString> SelectedStorageOption;
    TSharedPtr<FString> SelectedSyncOption;

    TSharedRef<ITableRow> MakeRowWidget(FGameDepotAssetRowPtr Row, const TSharedRef<STableViewBase>& OwnerTable);
    TSharedRef<SWidget> BuildTopBar();
    TSharedRef<SWidget> BuildSummaryBar();
    TSharedRef<SWidget> BuildFilterCombo(TArray<TSharedPtr<FString>>& Options, bool bStorageFilterCombo);
    TSharedRef<SWidget> BuildRuleButtons();
    TSharedRef<SWidget> BuildLegend() const;
    TSharedRef<SWidget> BuildDetailsPanel() const;

    void ApplyFilters();
    bool PassesFilters(const FGameDepotAssetRowPtr& Row) const;
    FText GetSummaryText() const;
    FText GetSelectionText() const;
    TArray<FString> GetSelectedDepotPaths() const;
    FString GetFirstSelectedDepotPath() const;
    void ShowHistoryForSelected();
    void RevertSelectedUncommitted();
    void RequestRuleForSelected(const FString& Mode);
};
