import React, { useEffect, useMemo, useState } from 'react';
import {
  Button,
  Descriptions,
  Empty,
  Modal,
  Space,
  Spin,
  Tabs,
  Typography,
} from '@douyinfe/semi-ui';
import { copy, showError, showSuccess } from '../../helpers';

const { TabPane } = Tabs;

const RELATED_FILTER_ALL = 'all';

function getRelatedCategory(routeGroup) {
  if (!routeGroup) {
    return 'other';
  }
  if (routeGroup.includes('submit')) {
    return 'submit';
  }
  if (routeGroup.includes('fetch')) {
    return 'fetch';
  }
  if (routeGroup.includes('content')) {
    return 'content';
  }
  if (routeGroup.includes('notify')) {
    return 'notify';
  }
  if (routeGroup.includes('seed')) {
    return 'seed';
  }
  return 'other';
}

function getRelatedCategoryLabel(category, t) {
  switch (category) {
    case RELATED_FILTER_ALL:
      return t('全部');
    case 'submit':
      return t('提交');
    case 'fetch':
      return t('查询');
    case 'content':
      return t('内容');
    case 'notify':
      return t('回调');
    case 'seed':
      return t('种子');
    default:
      return t('其他');
  }
}

function renderRelatedTitle(record, t) {
  if (!record) {
    return '-';
  }
  const routeGroup = record.route_group || '-';
  const method = record.method || '-';
  const statusCode = record.status_code ?? '-';
  return `${routeGroup} · ${method} · ${statusCode}`;
}

function getModelMappingValue(record, t) {
  if (!record) {
    return '-';
  }
  const requestedModel = record.model_name || '';
  const upstreamModel =
    record.upstream_model_name || record?.trace?.model_resolution?.upstream_model || '';
  const isModelMapped =
    record?.trace?.model_resolution?.is_model_mapped ||
    (requestedModel && upstreamModel && requestedModel !== upstreamModel);
  if (!requestedModel && !upstreamModel) {
    return '-';
  }
  if (!isModelMapped || !upstreamModel || requestedModel === upstreamModel) {
    return t('未发生映射');
  }
  return `${requestedModel} -> ${upstreamModel}`;
}

function stringifyValue(value) {
  if (value === undefined || value === null || value === '') {
    return '';
  }
  if (typeof value === 'string') {
    return value;
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch (error) {
    return String(value);
  }
}

function JsonBlock({ value }) {
  const content = stringifyValue(value);
  if (!content) {
    return <Empty description='无数据' image={null} />;
  }
  return (
    <pre
      style={{
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        lineHeight: 1.6,
        maxHeight: 'min(48vh, 520px)',
        overflow: 'auto',
        background: 'var(--semi-color-fill-0)',
        padding: 16,
        borderRadius: 12,
        margin: 0,
      }}
    >
      {content}
    </pre>
  );
}

const RequestAuditModal = ({
  visible,
  onCancel,
  loading,
  auditRecord,
  onOpenRequestAudit,
  t,
}) => {
  const [relatedFilter, setRelatedFilter] = useState(RELATED_FILTER_ALL);

  useEffect(() => {
    setRelatedFilter(RELATED_FILTER_ALL);
  }, [auditRecord?.request_id, visible]);

  const relatedFilters = useMemo(() => {
    const records = Array.isArray(auditRecord?.related_records)
      ? auditRecord.related_records
      : [];
    const counters = new Map([[RELATED_FILTER_ALL, records.length]]);
    records.forEach((record) => {
      const category = getRelatedCategory(record.route_group);
      counters.set(category, (counters.get(category) || 0) + 1);
    });
    return Array.from(counters.entries()).map(([key, count]) => ({
      key,
      count,
      label: getRelatedCategoryLabel(key, t),
    }));
  }, [auditRecord?.related_records, t]);

  const filteredRelatedRecords = useMemo(() => {
    const records = Array.isArray(auditRecord?.related_records)
      ? auditRecord.related_records
      : [];
    if (relatedFilter === RELATED_FILTER_ALL) {
      return records;
    }
    return records.filter(
      (record) => getRelatedCategory(record.route_group) === relatedFilter,
    );
  }, [auditRecord?.related_records, relatedFilter]);

  const handleCopySection = async (label, value) => {
    const content = stringifyValue(value);
    if (!content) {
      showError(t('当前没有可复制的内容'));
      return;
    }
    if (await copy(content)) {
      showSuccess(`${t('已复制')} ${label}`);
      return;
    }
    showError(t('复制失败，请手动复制'));
  };

  const summaryData = auditRecord
    ? [
        { key: t('Request ID'), value: auditRecord.request_id || '-' },
        { key: t('模式'), value: auditRecord.mode || '-' },
        { key: t('分组'), value: auditRecord.group || '-' },
        { key: t('路由类型'), value: auditRecord.route_group || '-' },
        { key: t('请求路径'), value: auditRecord.route_path || '-' },
        { key: t('请求方法'), value: auditRecord.method || '-' },
        { key: t('状态码'), value: auditRecord.status_code ?? '-' },
        { key: t('成功'), value: auditRecord.success ? t('是') : t('否') },
        { key: t('模型'), value: auditRecord.model_name || '-' },
        {
          key: t('上游模型'),
          value: auditRecord.upstream_model_name || '-',
        },
        {
          key: t('模型映射'),
          value: getModelMappingValue(auditRecord, t),
        },
        { key: t('渠道'), value: auditRecord.channel_name || '-' },
        { key: t('令牌'), value: auditRecord.token_name || '-' },
        { key: t('任务ID'), value: auditRecord.task_id || '-' },
        { key: t('MjID'), value: auditRecord.mj_id || '-' },
        { key: t('流式'), value: auditRecord.is_stream ? t('是') : t('否') },
        {
          key: t('Playground'),
          value: auditRecord.is_playground ? t('是') : t('否'),
        },
        { key: t('总耗时'), value: `${auditRecord.latency_ms || 0} ms` },
        {
          key: t('首包耗时'),
          value:
            auditRecord.first_response_ms && auditRecord.first_response_ms > 0
              ? `${auditRecord.first_response_ms} ms`
              : '-',
        },
        { key: t('重试次数'), value: auditRecord.retry_count || 0 },
      ]
    : [];

  return (
    <Modal
      title={t('请求审计详情')}
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width='min(1120px, calc(100vw - 32px))'
      bodyStyle={{
        maxHeight: 'calc(78vh - 96px)',
        overflowY: 'auto',
        paddingTop: 8,
        paddingBottom: 12,
      }}
      centered
    >
      <Spin spinning={loading}>
        {!auditRecord ? (
          <Empty description={t('暂无审计详情')} image={null} />
        ) : (
          <>
            <Descriptions data={summaryData} columns={2} />
            {Array.isArray(auditRecord.related_records) &&
            auditRecord.related_records.length > 0 ? (
              <div style={{ marginTop: 16 }}>
                <Typography.Text
                  type='secondary'
                  style={{ display: 'block', marginBottom: 8 }}
                >
                  {t('关联请求')}
                </Typography.Text>
                <Space wrap style={{ marginBottom: 8 }}>
                  {relatedFilters.map((filter) => (
                    <Button
                      key={filter.key}
                      size='small'
                      type={
                        relatedFilter === filter.key ? 'primary' : 'tertiary'
                      }
                      onClick={() => setRelatedFilter(filter.key)}
                    >
                      {`${filter.label} (${filter.count})`}
                    </Button>
                  ))}
                </Space>
                <Space wrap>
                  {filteredRelatedRecords.map((record) => (
                    <Button
                      key={record.request_id}
                      size='small'
                      type={
                        record.request_id === auditRecord.request_id
                          ? 'primary'
                          : 'tertiary'
                      }
                      disabled={
                        !onOpenRequestAudit ||
                        record.request_id === auditRecord.request_id
                      }
                      onClick={() => onOpenRequestAudit?.(record.request_id)}
                    >
                      {renderRelatedTitle(record, t)}
                    </Button>
                  ))}
                </Space>
              </div>
            ) : null}
            <Space wrap style={{ marginTop: 16 }}>
              <Button
                size='small'
                type='tertiary'
                onClick={() => handleCopySection(t('请求内容'), auditRecord.request)}
              >
                {t('复制请求')}
              </Button>
              <Button
                size='small'
                type='tertiary'
                onClick={() => handleCopySection(t('响应内容'), auditRecord.response)}
              >
                {t('复制响应')}
              </Button>
              <Button
                size='small'
                type='tertiary'
                onClick={() => handleCopySection(t('链路内容'), auditRecord.trace)}
              >
                {t('复制链路')}
              </Button>
              <Button
                size='small'
                type='tertiary'
                onClick={() =>
                  handleCopySection(t('请求 ID'), auditRecord.request_id || '')
                }
              >
                {t('复制 Request ID')}
              </Button>
            </Space>
            <Tabs type='line' style={{ marginTop: 16 }}>
              <TabPane tab={t('请求')} itemKey='request'>
                <JsonBlock value={auditRecord.request} />
              </TabPane>
              <TabPane tab={t('响应')} itemKey='response'>
                <JsonBlock value={auditRecord.response} />
              </TabPane>
              <TabPane tab={t('链路')} itemKey='trace'>
                <JsonBlock value={auditRecord.trace} />
              </TabPane>
              <TabPane tab={t('原始概览')} itemKey='summary'>
                <Typography.Text type='tertiary' style={{ display: 'block', marginBottom: 12 }}>
                  {t('以下内容为审计记录的基础字段快照')}
                </Typography.Text>
                <JsonBlock
                  value={{
                    request_id: auditRecord.request_id,
                    route_group: auditRecord.route_group,
                    route_path: auditRecord.route_path,
                    method: auditRecord.method,
                    status_code: auditRecord.status_code,
                    success: auditRecord.success,
                    relay_format: auditRecord.relay_format,
                    relay_mode: auditRecord.relay_mode,
                    model_name: auditRecord.model_name,
                    upstream_model_name: auditRecord.upstream_model_name,
                    group: auditRecord.group,
                    token_id: auditRecord.token_id,
                    token_name: auditRecord.token_name,
                    channel_id: auditRecord.channel_id,
                    channel_name: auditRecord.channel_name,
                    channel_type: auditRecord.channel_type,
                    task_id: auditRecord.task_id,
                    mj_id: auditRecord.mj_id,
                    latency_ms: auditRecord.latency_ms,
                    first_response_ms: auditRecord.first_response_ms,
                    retry_count: auditRecord.retry_count,
                  }}
                />
              </TabPane>
            </Tabs>
          </>
        )}
      </Spin>
    </Modal>
  );
};

export default RequestAuditModal;
