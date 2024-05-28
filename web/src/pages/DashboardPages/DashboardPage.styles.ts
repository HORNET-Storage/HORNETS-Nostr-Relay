import styled from 'styled-components';
import { LAYOUT, media } from '@app/styles/themes/constants';
import { BaseCol } from '@app/components/common/BaseCol/BaseCol';

export const RightSideCol = styled(BaseCol)`
  padding: ${LAYOUT.desktop.paddingVertical} ${LAYOUT.desktop.paddingHorizontal};
  position: sticky;
  top: 0;
  display: flex;
  flex-direction: column;
  height: calc(100vh - ${LAYOUT.desktop.headerHeight});
  background-color: var(--sider-background-color);
  overflow-y: auto;
  &::-webkit-scrollbar {
    width: 6px;
    display: none; // Hide scrollbar by default
  }
`;

export const LeftSideCol = styled(BaseCol)`
  @media only screen and ${media.xl} {
    padding: ${LAYOUT.desktop.paddingVertical} ${LAYOUT.desktop.paddingHorizontal};
    height: calc(100vh - ${LAYOUT.desktop.headerHeight});
    overflow: auto;
    &::-webkit-scrollbar {
          width: 6px;
          display: none; // Hide scrollbar by default
        }
  }
`;

// export const LeftSideCol = styled(BaseCol)`
//   padding: ${LAYOUT.desktop.paddingVertical} ${LAYOUT.desktop.paddingHorizontal};
//   position: sticky;
//   top: 0;
//   display: flex;
//   flex-direction: column;
//   height: calc(100vh - ${LAYOUT.desktop.headerHeight});
//   background-color: var(--sider-background-color);
//   overflow-y: auto;
//   overflow-x: hidden;

//   &::-webkit-scrollbar {
//     width: 6px;
//     display: none; // Hide scrollbar by default
//   }

//   -ms-overflow-style: none; // IE and Edge
//   scrollbar-width: none; // Firefox

//   // &.scrolling {
//   //   &::-webkit-scrollbar {
//   //     display: block;
//   //   }
//   //   scrollbar-width: thin; // Enable scrollbar for Firefox
//   //   -ms-overflow-style: auto; // Enable scrollbar for IE and Edge
//   // }
// `;

export const Space = styled.div`
  margin: 1rem 0;
`;

export const ScrollWrapper = styled.div`
  overflow-y: auto;
  overflow-x: hidden;
  min-height: 250px;

  .ant-card-body {
    overflow-y: auto;
    overflow-x: hidden;
    height: 100%;
  }

  // Hide scrollbars for WebKit browsers (e.g., Chrome, Safari)
  &::-webkit-scrollbar {
    display: none;
  }

  // Hide scrollbars for Firefox
  scrollbar-width: none; // Firefox

  // Hide scrollbars for IE, Edge
  -ms-overflow-style: none;
    
`;

export const BlockWrapper = styled.div`
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
  gap: 15px;

  background: black;

  min-height: 300px;
  overflow-y: auto;
`;

export const Item = styled.div`
  background: red;
  height: 150px;
  flex-shrink: 0;
`;

// export const ScrollWrapper = styled.div`
//   overflow-y: auto;
//   overflow-x: hidden;
//   min-height: 250px;

//   &.scrolling {
//     &::-webkit-scrollbar {
//       display: block;
//     }
    
//     scrollbar-width: thin;
//     -ms-overflow-style: auto;
//   }

//   &::-webkit-scrollbar {
//     width: 6px;
//     display: none; // Default state hides the scrollbar
//   }

//   -ms-overflow-style: none; // IE and Edge
//   scrollbar-width: none; // Firefox
// `;
