# frozen_string_literal: true

module Releases
  class CreateService < Releases::BaseService
    def execute
      return error(_('Access Denied'), 403) unless allowed?
      return error(_('You are not allowed to create this tag as it is protected.'), 403) unless can_create_tag?
      return error(_('Release already exists'), 409) if release
      return error(format(_("Milestone(s) not found: %{milestones}"), milestones: inexistent_milestone_titles.join(', ')), 400) if inexistent_milestone_titles.any? # rubocop:disable Layout/LineLength
      return error(format(_("Milestone id(s) not found: %{milestones}"), milestones: inexistent_milestone_ids.join(', ')), 400) if inexistent_milestone_ids.any? # rubocop:disable Layout/LineLength

      # should be found before the creation of new tag
      # because tag creation can spawn new pipeline
      # which won't have any data for evidence yet
      evidence_pipeline = Releases::EvidencePipelineFinder.new(project, params).execute

      tag = ensure_tag

      return tag unless tag.is_a?(Gitlab::Git::Tag)

      if project.catalog_resource
        response = Ci::Catalog::Resources::ValidateService.new(project, ref).execute

        return error(response.message) if response.error?
      end

      create_release(tag, evidence_pipeline)
    end

    private

    def ensure_tag
      existing_tag || create_tag
    end

    def create_tag
      return error('Ref is not specified', 422) unless ref

      result = Tags::CreateService
        .new(project, current_user)
        .execute(tag_name, ref, tag_message)

      return result unless result[:status] == :success

      result[:tag]
    end

    def allowed?
      Ability.allowed?(current_user, :create_release, project)
    end

    def can_create_tag?
      ::Gitlab::UserAccess.new(current_user, container: project).can_create_tag?(tag_name)
    end

    def create_release(tag, evidence_pipeline)
      release = build_release(tag)

      release.save!

      notify_create_release(release)

      execute_hooks(release, 'create')

      create_evidence!(release, evidence_pipeline)

      audit(release, action: :created)

      success(tag: tag, release: release)
    rescue StandardError => e
      error(e.message, 400)
    end

    def notify_create_release(release)
      NotificationService.new.async.send_new_release_notifications(release)
    end

    def build_release(tag)
      project.releases.build(
        name: name,
        description: description,
        author: current_user,
        tag: tag.name,
        sha: tag.dereferenced_target.sha,
        released_at: released_at,
        links_attributes: links_attributes,
        milestones: milestones
      )
    end

    def links_attributes
      (params.dig(:assets, 'links') || []).map do |link_params|
        Releases::Links::Params.new(link_params).allowed_params
      end
    end

    def create_evidence!(release, pipeline)
      return if release.historical_release? || release.upcoming_release?

      ::Releases::CreateEvidenceWorker.perform_async(release.id, pipeline&.id)
    end
  end
end
