import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BaseCard } from '../../common/BaseCard/BaseCard';
import { ActivityChart } from './ActivityChart';
// import { ChartData } from 'interfaces/interfaces';
import styled from 'styled-components';

export const ActivityCard: React.FC = () => {
  const { t } = useTranslation();

  return (
    <ActivityCardStyled id="activity" title={t('medical-dashboard.activity.title')} padding={0}>
      <ActivityChart />
    </ActivityCardStyled>
  );
};

const ActivityCardStyled = styled(BaseCard)`
  height: 100%;
`;
