// eslint-disable-next-line
import React from 'react';
import { useTranslation } from 'react-i18next';
import styled from 'styled-components';
import { BaseSwitch } from '@app/components/common/BaseSwitch/BaseSwitch';
import { BaseSelect, Option } from '@app/components/common/selects/BaseSelect/BaseSelect';
import { useAppDispatch, useAppSelector } from '@app/hooks/reduxHooks';
import { setMode, setKinds, setMediaTypes } from '@app/store/slices/modeSlice';

const SwitchContainer = styled.div`
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
`;

export const SmartBaseModeSettings: React.FC = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const mode = useAppSelector((state) => state.mode.relayMode);
  const kinds = useAppSelector((state) => state.mode.kinds);

  const handleModeChange = () => {
    dispatch(setMode(mode === 'smart' ? 'unlimited' : 'smart'));
  };

  // Example kinds options, adjust as necessary
  const kindOptions = [
    { value: 0, label: 'Kind 0' },
    { value: 1, label: 'Kind 1' },
    { value: 3, label: 'Kind 3' },
    { value: 5, label: 'Kind 5' },
    { value: 11, label: 'Kind 11' },
  ];

  return (
    <div>
      <SwitchContainer>
        <span>{t('common.serverSetting')}</span>
        <BaseSwitch
          checkedChildren="Smart"
          unCheckedChildren="unlimited"
          checked={mode === 'smart'}
          onChange={handleModeChange}
        />
      </SwitchContainer>
      <h4>{mode === 'smart' ? t('common.supportedKindsAndMedia') : t('common.unsupportedKindsAndMedia')}</h4>
        <>
          <BaseSelect
            mode="multiple"
            placeholder={t('common.setKinds')}
            defaultValue={kinds.map((k) => k.toString())}
            onChange={(selectedKinds: any) => dispatch(setKinds((selectedKinds as string[]).map(Number)))}
            style={{ width: '100%' }}
          >
            {kindOptions.map((option) => (
              <Option key={option.value} value={option.value.toString()}>
                {option.label}
              </Option>
            ))}
          </BaseSelect>
          <BaseSelect
            mode="multiple"
            placeholder={t('common.setMediaTypes')}
            onChange={(selectedTypes: any) => dispatch(setMediaTypes(selectedTypes as string[]))}
            style={{ width: '100%', marginTop: '10px' }}
          >
            <Option value="video">Video</Option>
            <Option value="audio">Audio</Option>
            <Option value="image">Images</Option>
          </BaseSelect>
        </>
    </div>
  );
};