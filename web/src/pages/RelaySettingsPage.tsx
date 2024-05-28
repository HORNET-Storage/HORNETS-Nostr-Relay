import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { BaseSwitch } from '@app/components/common/BaseSwitch/BaseSwitch';
import { BaseCheckbox } from '@app/components/common/BaseCheckbox/BaseCheckbox';
import { useAppDispatch, useAppSelector } from '@app/hooks/reduxHooks';
import { setMode } from '@app/store/slices/modeSlice';
import { PageTitle } from '@app/components/common/PageTitle/PageTitle';
import { BaseButton } from '@app/components/common/BaseButton/BaseButton';
import { BaseRow } from '@app/components/common/BaseRow/BaseRow';
import { BaseCol } from '@app/components/common/BaseCol/BaseCol';
import { Balance } from '@app/components/nft-dashboard/Balance/Balance';
import { TotalEarning } from '@app/components/nft-dashboard/totalEarning/TotalEarning';
import { ActivityStory } from '@app/components/nft-dashboard/activityStory/ActivityStory';
import * as S from '@app/pages/uiComponentsPages//UIComponentsPage.styles';
import useRelaySettings from '@app/hooks/useRelaySettings';

const RelaySettingsPage: React.FC = () => {
  const [loadings, setLoadings] = useState<boolean[]>([]);
  const enterLoading = (index: number) => {
    setLoadings(loadings => {
      const newLoadings = [...loadings];
      newLoadings[index] = true;
      return newLoadings;
    });
  };

  const exitLoading = (index: number) => {
    setLoadings(loadings => {
      const newLoadings = [...loadings];
      newLoadings[index] = false;
      return newLoadings;
    });
  };

  const { relaySettings, updateSettings, saveSettings } = useRelaySettings();
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const relaymode = useAppSelector(state => state.mode.relayMode);

  const noteOptions = [
    'kind0', 'kind1', 'kind3', 'kind10000', 'kind1984', 'kind30000', 'kind30008',
    'kind30009', 'kind30023', 'kind36810', 'kind5', 'kind6', 'kind7', 'kind8',
    'kind9372', 'kind9373', 'kind9735', 'kind9802'
  ].map(kind => ({
    label: <S.CheckboxLabel isActive={relaySettings.isKindsActive}>{t(`checkboxes.${kind}`)}</S.CheckboxLabel>,
    value: kind
  }));

  const photoFormatOptions = [
    'jpeg', 'jpg', 'png', 'gif', 'bmp', 'tiff', 'raw',
    'svg', 'eps', 'psd', 'ai', 'pdf', 'webp'
  ].map(format => ({
    label: <S.CheckboxLabel isActive={relaySettings.isPhotosActive}>{t(`checkboxes.${format}`)}</S.CheckboxLabel>,
    value: format
  }));

  const videoFormatOptions = [
    'avi', 'mp4', 'mov', 'wmv', 'mkv', 'flv',
    'mpeg', '3gp', 'webm', 'ogg'
  ].map(format => ({
    label: <S.CheckboxLabel isActive={relaySettings.isVideosActive}>{t(`checkboxes.${format}`)}</S.CheckboxLabel>,
    value: format
  }));

  const gitNestrHkindOptions = [
    'Nostr/file_attachment', 'GitNestr/bundle_file', 'GitNestr/archive_repo'
  ].map(hkind => ({
    label: <S.CheckboxLabel isActive={relaySettings.isGitNestrActive}>{t(`checkboxes.${hkind}`)}</S.CheckboxLabel>,
    value: hkind
  }));

  const handleModeChange = (checked: boolean) => {
    const newMode = checked ? 'smart' : 'unlimited';
    updateSettings('mode', newMode);
    dispatch(setMode(newMode));
  };

  // Define a type for the local state that matches the RelaySettings type
  type Settings = {
    mode: string;
    kinds: string[];
    photos: string[];
    videos: string[];
    gitNestr: string[];
    isKindsActive: boolean;
    isPhotosActive: boolean;
    isVideosActive: boolean;
    isGitNestrActive: boolean;
  };

  const [settings, setSettings] = useState<Settings>({
    mode: 'unlimited',
    kinds: [],
    photos: [],
    videos: [],
    gitNestr: [],
    isKindsActive: true,
    isPhotosActive: true,
    isVideosActive: true,
    isGitNestrActive: true
  });

  type Category = 'kinds' | 'photos' | 'videos' | 'gitNestr';

  const handleSettingsChange = (category: Category, checkedValues: string[]) => {
    setSettings(prevSettings => ({
      ...prevSettings,
      [category]: checkedValues
    }));
    updateSettings(category, checkedValues);
  };

  const handleSwitchChange = (category: keyof Settings, value: boolean) => {
    setSettings(prevSettings => ({
      ...prevSettings,
      [category]: value
    }));
    updateSettings(category, value);
  };

  const onSaveClick = async () => {
    enterLoading(0);

    if (!settings.isKindsActive) {
      handleSettingsChange('kinds', []);
    }
    if (!settings.isPhotosActive) {
      handleSettingsChange('photos', []);
    }
    if (!settings.isVideosActive) {
      handleSettingsChange('videos', []);
    }
    if (!settings.isGitNestrActive) {
      console.log('gitNestr is not active');
      handleSettingsChange('gitNestr', []);
    }

    // Call updateSettings before calling saveSettings to ensure settings state is updated correctly
    await Promise.all([
      updateSettings('kinds', settings.isKindsActive ? settings.kinds : []),
      updateSettings('photos', settings.isPhotosActive ? settings.photos : []),
      updateSettings('videos', settings.isVideosActive ? settings.videos : []),
      updateSettings('gitNestr', settings.isGitNestrActive ? settings.gitNestr : [])
    ]);

    await saveSettings();
    
    exitLoading(0);
  };

  useEffect(() => {
    if (relaySettings) {
      setSettings(relaySettings);
    }
  }, [relaySettings]);

  return (
    <>
      <PageTitle>{t('common.customizeRelaySettings')}</PageTitle>
      <BaseRow gutter={[60, 60]}>
        <S.LeftSideCol xl={16} xxl={17} id="desktop-content">
          <BaseCol xs={24}>
            <S.SwitchContainer>
              <S.LabelSpan>{t('common.serverSetting')}</S.LabelSpan>
              <S.LargeSwitch
                checkedChildren="smart"
                unCheckedChildren="unlimited"
                checked={relaymode === 'smart'}
                onChange={(e) => handleModeChange(e)}
              />
            </S.SwitchContainer>
          </BaseCol>

          <BaseCol xs={24}>
            <S.Card title={t('checkboxes.kinds')}>
              <BaseCol xs={24}>
                <BaseSwitch
                  checkedChildren="ON"
                  unCheckedChildren="OFF"
                  checked={settings.isKindsActive}
                  onChange={() => handleSwitchChange('isKindsActive', !settings.isKindsActive)}
                />
              </BaseCol>

              <BaseCheckbox.Group
                options={noteOptions}
                value={settings.kinds}
                onChange={(checkedValues) => handleSettingsChange('kinds', checkedValues as string[])}
                disabled={!settings.isKindsActive}
              />
            </S.Card>
          </BaseCol>

          <BaseCol xs={24}>
            <S.Card title={t('checkboxes.photos')}>
              <BaseCol xs={24}>
                <BaseSwitch
                  checkedChildren="ON"
                  unCheckedChildren="OFF"
                  checked={settings.isPhotosActive}
                  onChange={() => handleSwitchChange('isPhotosActive', !settings.isPhotosActive)}
                />
              </BaseCol>

              <BaseCheckbox.Group
                options={photoFormatOptions}
                value={settings.photos}
                onChange={(checkedValues) => handleSettingsChange('photos', checkedValues as string[])}
                disabled={!settings.isPhotosActive}
              />
            </S.Card>
          </BaseCol>

          <BaseCol xs={24}>
            <S.Card title={t('checkboxes.videos')}>
              <BaseCol xs={24}>
                <BaseSwitch
                  checkedChildren="ON"
                  unCheckedChildren="OFF"
                  checked={settings.isVideosActive}
                  onChange={() => handleSwitchChange('isVideosActive', !settings.isVideosActive)}
                />
              </BaseCol>

              <BaseCheckbox.Group
                options={videoFormatOptions}
                value={settings.videos}
                onChange={(checkedValues) => handleSettingsChange('videos', checkedValues as string[])}
                disabled={!settings.isVideosActive}
              />
            </S.Card>
          </BaseCol>

          <BaseCol xs={24}>
            <S.Card title={t('checkboxes.gitNestr')}>
              <BaseCol xs={24}>
                <BaseSwitch
                  checkedChildren="ON"
                  unCheckedChildren="OFF"
                  checked={settings.isGitNestrActive}
                  onChange={() => handleSwitchChange('isGitNestrActive', !settings.isGitNestrActive)}
                />
              </BaseCol>
              <BaseCheckbox.Group
                options={gitNestrHkindOptions}
                value={settings.gitNestr}
                onChange={(checkedValues) => handleSettingsChange('gitNestr', checkedValues as string[])}
                disabled={!settings.isGitNestrActive}
              />
            </S.Card>
            <BaseButton type="primary" loading={loadings[0]} onClick={onSaveClick}>
              {t('buttons.saveSettings')}
            </BaseButton>
          </BaseCol>
        </S.LeftSideCol>
        <S.RightSideCol xl={8} xxl={7}>
          <div id="balance">
            <Balance />
          </div>
          <S.Space />
          <div id="total-earning">
            <TotalEarning />
          </div>
          <S.Space />
          <S.ScrollWrapper id="activity-story">
            <ActivityStory />
          </S.ScrollWrapper>
        </S.RightSideCol>
      </BaseRow>
    </>
  );
};

export default RelaySettingsPage;
